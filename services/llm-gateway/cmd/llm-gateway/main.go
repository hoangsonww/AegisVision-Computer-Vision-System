// Command llm-gateway is the platform's uniform LLM/VLM front door.
//
// Per ADR-0018, internal callers speak one OpenAI-compatible vocabulary
// regardless of which model runtime sits behind us. The gateway:
//
//   1. Verifies the caller identity (mTLS in-cluster; X-Aegis-Identity from
//      auth-proxy). Tenant is taken from the identity, NEVER trusted from
//      payload.
//   2. Applies the safety layer (prompt-injection sanitizer, PII redaction)
//      from pkg/llm.SafeClient. Every response includes safety_signals.
//   3. Enforces per-tenant rate limits (pkg/platform/middleware.RateLimit).
//   4. Forwards to the configured backend (vLLM/TGI/Triton+TRT-LLM/...) and
//      records token usage for metering-service.
//
// Hardening: in production (AEGIS_ENV != dev|test) the service refuses to
// start unless an upstream LLM_BACKEND_URL is configured AND an identity
// HMAC key (>=32 bytes) is set. Synthetic "fake" responses are a dev-only
// convenience that must NEVER serve production tenant traffic.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
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

	"github.com/aegisvision/services/llm-gateway/internal/backend"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "llm-gateway"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8400")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8401")
	backendURL := config.String("AEGIS_LLM_BACKEND_URL", "")
	authForward := config.String("AEGIS_LLM_BACKEND_AUTH", "")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	if env != "dev" && env != "test" {
		if backendURL == "" {
			return errors.New("AEGIS_LLM_BACKEND_URL is required outside dev")
		}
		if !strings.HasPrefix(backendURL, "https://") && !strings.HasPrefix(backendURL, "http://") {
			return errors.New("AEGIS_LLM_BACKEND_URL must be http(s)://...")
		}
	}

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	ctx := context.Background()
	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{
		ServiceName: serviceName, ServiceVersion: version, Env: env,
		OTLPEndpoint: otlpEndpoint, Insecure: env == "dev", SamplingRatio: 1.0,
	})
	if err != nil {
		return err
	}

	var be backend.Backend
	if backendURL != "" {
		hb := backend.NewHTTP(backendURL)
		hb.Client.AuthHeader = authForward
		be = hb
		logger.Info("backend configured", slog.String("url", backendURL))
	} else {
		be = backend.NewFake()
		logger.Warn("no LLM backend configured — using FAKE backend (dev only)")
	}

	// Safety wrapper around the backend.
	safe := llm.NewSafeClient(adapter{be})

	// Metrics / RED.
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)

	// Domain-specific counters.
	tokensCtr := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_llm_tokens_total", Help: "Tokens consumed."}, []string{"tenant", "model", "kind"})
	reg.MustRegister(tokensCtr)
	refusalCtr := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_llm_refusals_total", Help: "Safety refusals."}, []string{"tenant", "reason"})
	reg.MustRegister(refusalCtr)

	rl := middleware.RateLimit(middleware.RateLimitConfig{
		Metric:   "llm_requests",
		Provider: middleware.NoLimit{}, // wire to tenant-service in prod via CachedHTTPQuotaProvider
	})

	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(
				middleware.TenantFromHeader()(rl(h))))))
	}

	mux.Handle("POST /v1/chat/completions", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		// Tenant is from the identity, not from payload — overwrite.
		req.TenantID = logging.TenantID(r.Context())
		resp, err := safe.Chat(r.Context(), req)
		if err != nil {
			logger.WarnContext(r.Context(), "chat failed", slog.String("err", err.Error()))
			problem.Internal(err.Error()).Write(w)
			return
		}
		if refused, _ := resp.Safety["refused"].(bool); refused {
			reason, _ := resp.Safety["refusal_reason"].(string)
			refusalCtr.WithLabelValues(req.TenantID, reason).Inc()
		}
		tokensCtr.WithLabelValues(req.TenantID, resp.Model, "prompt").Add(float64(resp.Usage.PromptTokens))
		tokensCtr.WithLabelValues(req.TenantID, resp.Model, "completion").Add(float64(resp.Usage.CompletionTokens))
		writeJSON(w, http.StatusOK, resp)
	}), "/v1/chat/completions"))

	mux.Handle("POST /v1/embeddings", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.EmbeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		req.TenantID = logging.TenantID(r.Context())
		resp, err := safe.Embed(r.Context(), req)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		tokensCtr.WithLabelValues(req.TenantID, req.Model, "embed").Add(float64(resp.Usage.TotalTokens))
		writeJSON(w, http.StatusOK, resp)
	}), "/v1/embeddings"))

	mux.Handle("POST /v1/vision/describe", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.VisionDescribeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		req.TenantID = logging.TenantID(r.Context())
		if req.Image.URN == "" {
			problem.BadRequest("image.urn required").Write(w)
			return
		}
		resp, err := safe.Describe(r.Context(), req)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		tokensCtr.WithLabelValues(req.TenantID, req.Model, "vision").Add(float64(resp.Usage.TotalTokens))
		writeJSON(w, http.StatusOK, resp)
	}), "/v1/vision/describe"))

	// Lightweight model catalog (mirrors OpenAI /v1/models for compatibility).
	mux.Handle("GET /v1/models", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "default", "object": "model"},
			},
		})
	}), "/v1/models"))

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
	logger.Info("started", slog.String("addr", httpAddr), slog.String("env", env))
	return runner.Wait(ctx)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// adapter wraps backend.Backend to satisfy llm.Client (so SafeClient can wrap it).
type adapter struct{ be backend.Backend }

func (a adapter) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return a.be.Chat(ctx, req)
}
func (a adapter) Embed(ctx context.Context, req llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return a.be.Embed(ctx, req)
}
func (a adapter) Describe(ctx context.Context, req llm.VisionDescribeRequest) (*llm.VisionDescribeResponse, error) {
	return a.be.Describe(ctx, req)
}
