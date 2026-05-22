// Command nlq-service translates natural-language prompts to executable
// reads against the platform (event search) or to draft DAGs for the user
// to review. Read-only by design: pipelines never mutate here. Promotion
// is an explicit, gated path through pipeline-service.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aegisvision/pkg/llm"
	"github.com/aegisvision/pkg/platform/config"
	"github.com/aegisvision/pkg/platform/health"
	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/middleware"
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/problem"
	"github.com/aegisvision/pkg/platform/shutdown"

	"github.com/aegisvision/services/nlq-service/internal/translator"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "nlq-service"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8450")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8451")
	llmURL := config.String("AEGIS_LLM_GATEWAY_URL", "")
	model := config.String("AEGIS_NLQ_MODEL", "default")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	ctx := context.Background()
	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{
		ServiceName: serviceName, ServiceVersion: version, Env: env,
		OTLPEndpoint: otlpEndpoint, Insecure: env == "dev", SamplingRatio: 1.0,
	})
	if err != nil {
		return err
	}

	var c llm.Client
	if llmURL == "" {
		c = llm.NewSafeClient(llm.NewFakeClient())
		logger.Warn("AEGIS_LLM_GATEWAY_URL empty — using fake (dev only)")
	} else {
		c = llm.NewSafeClient(llm.NewHTTPClient(llmURL))
	}
	tr := translator.New(c, model)

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	queries := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_nlq_queries_total", Help: "NLQ queries."}, []string{"kind"})
	rejected := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_nlq_rejected_total", Help: "NLQ rejections by safety/translator."})
	reg.MustRegister(queries, rejected)

	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(
				middleware.TenantFromHeader()(h)))))
	}

	mux.Handle("POST /v1/queries", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Prompt   string `json:"prompt"`
			TimeFrom string `json:"time_from"`
			TimeTo   string `json:"time_to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		if body.Prompt == "" {
			problem.BadRequest("prompt required").Write(w)
			return
		}
		tenant := logging.TenantID(r.Context())
		resp, err := tr.Translate(r.Context(), tenant, body.Prompt, body.TimeFrom, body.TimeTo)
		if err != nil {
			rejected.Inc()
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		queries.WithLabelValues(string(resp.Kind)).Inc()
		writeJSON(w, http.StatusOK, resp)
	}), "/v1/queries"))

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
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })

	go func() { _ = httpSrv.ListenAndServe() }()
	go func() { _ = hsrv.ListenAndServe() }()
	logger.Info("started", slog.String("addr", httpAddr))
	return runner.Wait(ctx)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
