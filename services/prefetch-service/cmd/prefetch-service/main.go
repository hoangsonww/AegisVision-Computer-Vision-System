// Command prefetch-service is the predictive cache warmer (ADR-0026).
//
// Inputs:
//   - detections.v1 — bumps the predictor's grid cell for the observed
//     (tenant, model) pair.
// Outputs:
//   - inference-router warm-up RPCs at a configurable horizon (default
//     5 minutes ahead). The warm-up endpoint should be cheap and
//     idempotent — we don't run actual inference.
//   - aegis_prefetch_* metrics.
//
// The predictor is conservative: we send warm-ups for the top N
// (tenant, model) pairs in the horizon-ahead cell, capped per cycle.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/pkg/platform/config"
	"github.com/aegisvision/pkg/platform/health"
	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/middleware"
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/shutdown"

	"github.com/aegisvision/services/prefetch-service/internal/predictor"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "prefetch-service"

type detectionEvent struct {
	TenantID string `json:"tenant_id"`
	ModelID  string `json:"model_id"`
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8500")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8501")
	natsURL := config.String("AEGIS_NATS_URL", "")
	inferURL := config.String("AEGIS_INFERENCE_ROUTER_URL", "")
	horizon := config.Duration("AEGIS_PREFETCH_HORIZON", 5*time.Minute)
	cycle := config.Duration("AEGIS_PREFETCH_CYCLE", 60*time.Second)
	topN := config.Int("AEGIS_PREFETCH_TOPN", 16)
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{
		ServiceName: serviceName, ServiceVersion: version, Env: env,
		OTLPEndpoint: otlpEndpoint, Insecure: env == "dev", SamplingRatio: 1.0,
	})
	if err != nil {
		return err
	}

	var sub bus.Subscriber
	var pub bus.Publisher
	if natsURL != "" {
		nc, err := bus.Connect(natsURL)
		if err != nil {
			return err
		}
		sub, pub = nc, nc
	} else {
		mem := bus.NewInMemory()
		sub, pub = mem, mem
	}

	pred := predictor.New(0.2)

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	observed := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_prefetch_observed_total", Help: "Detections observed for prediction."})
	warmups := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_prefetch_warmups_total", Help: "Warm-up RPCs dispatched."}, []string{"outcome"})
	reg.MustRegister(observed, warmups)

	_, _ = sub.Subscribe(ctx, "detections.v1", "prefetch", func(ctx context.Context, msg bus.Message) error {
		var ev detectionEvent
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			return nil
		}
		if ev.TenantID == "" || ev.ModelID == "" {
			return nil
		}
		pred.Bump(time.Now().UTC(), predictor.Key{Tenant: ev.TenantID, Model: ev.ModelID})
		observed.Inc()
		return nil
	})

	httpc := &http.Client{Timeout: 3 * time.Second}
	go func() {
		t := time.NewTicker(cycle)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				preds := pred.Predict(horizon, topN)
				logger.Info("prefetch cycle", slog.Int("predictions", len(preds)))
				for _, p := range preds {
					if err := warm(ctx, httpc, inferURL, p.Key.Tenant, p.Key.Model); err != nil {
						warmups.WithLabelValues("error").Inc()
						continue
					}
					warmups.WithLabelValues("ok").Inc()
				}
				body, _ := json.Marshal(map[string]any{"cycle_at": time.Now().UTC(), "horizon": horizon.String(), "predictions": preds})
				_ = pub.Publish(ctx, bus.Message{Subject: "prefetch.cycle.v1", Data: body})
			}
		}
	}()

	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(h))))
	}

	// Debug endpoint: peek at the current top-N for an arbitrary horizon.
	mux.Handle("GET /v1/prefetch/predictions", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		preds := pred.Predict(horizon, topN)
		writeJSON(w, http.StatusOK, map[string]any{"horizon": horizon.String(), "top": preds})
	}), "/v1/prefetch/predictions"))

	httpSrv := &http.Server{Addr: httpAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	hr := health.NewRegistry()
	hmux := http.NewServeMux()
	hmux.Handle("/healthz", health.LivenessHandler())
	hmux.Handle("/readyz", hr.ReadinessHandler())
	hmux.Handle("/metrics", metrics.Handler(reg))
	hsrv := &http.Server{Addr: healthAddr, Handler: hmux, ReadHeaderTimeout: 5 * time.Second}

	runner := shutdown.New(logger)
	runner.Register("http", httpSrv.Shutdown)
	runner.Register("health-http", hsrv.Shutdown)
	runner.Register("bus", func(context.Context) error { return pub.Close() })
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })

	go func() { _ = httpSrv.ListenAndServe() }()
	go func() { _ = hsrv.ListenAndServe() }()
	logger.Info("started", slog.String("addr", httpAddr))
	return runner.Wait(ctx)
}

func warm(ctx context.Context, httpc *http.Client, base, tenant, model string) error {
	if base == "" {
		return nil // dev mode: skip but don't error
	}
	// Model IDs here follow the "model@version" convention. If the
	// predictor stored a bare model name (legacy events), we default
	// version to "1" — inference-router treats that as "latest".
	name, version := model, "1"
	for i := len(model) - 1; i >= 0; i-- {
		if model[i] == '@' {
			name = model[:i]
			version = model[i+1:]
			break
		}
	}
	body, _ := json.Marshal(map[string]string{"model": name, "version": version})
	req, _ := http.NewRequestWithContext(ctx, "POST", base+"/v1/models:warmup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Aegis-Tenant", tenant)
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
