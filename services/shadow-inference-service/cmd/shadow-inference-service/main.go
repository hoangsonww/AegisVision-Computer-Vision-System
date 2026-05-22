// Command shadow-inference-service runs the Phase-5 shadow comparator
// (ADR-0024). It subscribes to two bus subjects:
//
//   - frame.descriptor.* — the claim-check envelope for a single frame
//     (URN only; image bytes never travel the bus per ADR-0008)
//   - inference.baseline.v1 — the primary path's result for that frame
//
// When both arrive for the same frame URN, the engine picks a subset by
// the active canary's deterministic split and asks inference-router to
// run the *candidate* model on the same frame. The result is compared
// (label agreement) and accumulated into a per-window observation.
//
// Closed windows are flushed via /v1/canary/observations to the
// canary-controller, which folds them into its regression detector.
//
// Shadow inference NEVER publishes a tenant-facing detection. Its only
// outputs are observations + metrics.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/platform/config"
	"github.com/aegisvision/pkg/platform/health"
	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/middleware"
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/problem"
	"github.com/aegisvision/pkg/platform/shutdown"

	"github.com/aegisvision/services/shadow-inference-service/internal/shadow"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "shadow-inference-service"

type httpInvoker struct {
	BaseURL string
	HTTP    *http.Client
}

func (h *httpInvoker) Invoke(ctx context.Context, modelID, frameURN string) (shadow.BaselineResult, error) {
	// modelID is conventionally "model@version" (e.g. "yolo-v9@1.2"). If the
	// caller omitted "@", we default to version "1" — inference-router's
	// detector treats unspecified-version requests as "latest".
	model, version := modelID, "1"
	for i := len(modelID) - 1; i >= 0; i-- {
		if modelID[i] == '@' {
			model = modelID[:i]
			version = modelID[i+1:]
			break
		}
	}
	// Note: we send `frame_urn` so inference-router can dereference the
	// claim-check (ADR-0008). No frame bytes cross this RPC.
	body, _ := json.Marshal(map[string]any{
		"model":     model,
		"version":   version,
		"frame_urn": frameURN,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", h.BaseURL+"/v1/infer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return shadow.BaselineResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return shadow.BaselineResult{}, fmt.Errorf("infer %s: %s", resp.Status, string(b))
	}
	// inference-router emits {"detections": [...]} — we lift the first
	// detection's predicted_class as the shadow comparison signal.
	var rsp struct {
		Detections []struct {
			ClassLabel string  `json:"class_label"`
			Score      float64 `json:"score"`
		} `json:"detections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rsp); err != nil {
		return shadow.BaselineResult{}, err
	}
	if len(rsp.Detections) == 0 {
		return shadow.BaselineResult{Predicted: "", Confidence: 0}, nil
	}
	d := rsp.Detections[0]
	return shadow.BaselineResult{Predicted: d.ClassLabel, Confidence: d.Score}, nil
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
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8470")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8471")
	natsURL := config.String("AEGIS_NATS_URL", "")
	inferURL := config.String("AEGIS_INFERENCE_ROUTER_URL", "")
	canaryURL := config.String("AEGIS_CANARY_CONTROLLER_URL", "")
	flushEvery := config.Duration("AEGIS_SHADOW_FLUSH", 30*time.Second)
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

	var invoker shadow.CandidateInvoker
	if inferURL != "" {
		invoker = &httpInvoker{BaseURL: inferURL, HTTP: &http.Client{Timeout: 5 * time.Second}}
	}
	engine := shadow.NewEngine(invoker)

	var pub bus.Publisher
	var sub bus.Subscriber
	if natsURL != "" {
		nc, err := bus.Connect(natsURL)
		if err != nil {
			return err
		}
		pub, sub = nc, nc
	} else {
		mem := bus.NewInMemory()
		pub, sub = mem, mem
	}

	_, _ = sub.Subscribe(ctx, "frame.descriptor.>", "shadow-inference", func(ctx context.Context, msg bus.Message) error {
		fd, err := shadow.ParseFrame(msg.Data)
		if err != nil {
			return nil
		}
		engine.OnFrame(ctx, fd)
		return nil
	})
	_, _ = sub.Subscribe(ctx, "inference.baseline.v1", "shadow-inference", func(ctx context.Context, msg bus.Message) error {
		br, err := shadow.ParseBaseline(msg.Data)
		if err != nil {
			return nil
		}
		engine.OnBaseline(ctx, br)
		return nil
	})

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	flushedObs := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_shadow_observations_flushed_total", Help: "Observations flushed."}, []string{"variant"})
	disagreements := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_shadow_disagreements_total", Help: "Shadow disagreed with baseline."})
	reg.MustRegister(flushedObs, disagreements)

	// Periodic flush of closed windows to canary-controller.
	go func() {
		t := time.NewTicker(flushEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				obs := engine.Drain(time.Now().UTC())
				if len(obs) == 0 {
					continue
				}
				for _, o := range obs {
					if o.Variant == "candidate" {
						disagreements.Add(float64(o.Failures))
					}
					flushedObs.WithLabelValues(o.Variant).Inc()
				}
				if canaryURL != "" {
					_ = postObservations(ctx, canaryURL, obs)
				}
				// Also re-publish so other tools can subscribe.
				body, _ := json.Marshal(obs)
				_ = pub.Publish(ctx, bus.Message{Subject: "shadow.observations.v1", Data: body})
			}
		}
	}()

	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(h))))
	}

	// Operator-facing: register/replace the set of active shadow assignments.
	mux.Handle("PUT /v1/shadow/assignments", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []shadow.Active
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			problem.BadRequest("expect array").Write(w)
			return
		}
		engine.SetActive(body)
		w.WriteHeader(http.StatusNoContent)
	}), "/v1/shadow/assignments"))

	mux.Handle("GET /v1/shadow/observations:peek", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Peek (does not drain) is a debugging convenience: it advances
		// time far ahead to drain everything, but in production the
		// flusher does this on a schedule.
		obs := engine.Drain(time.Now().UTC().Add(time.Hour))
		writeJSON(w, http.StatusOK, obs)
	}), "/v1/shadow/observations:peek"))

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

func postObservations(ctx context.Context, base string, obs []intelligence.CanaryObservation) error {
	body, _ := json.Marshal(obs)
	req, _ := http.NewRequestWithContext(ctx, "POST", base+"/v1/canary/observations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
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
