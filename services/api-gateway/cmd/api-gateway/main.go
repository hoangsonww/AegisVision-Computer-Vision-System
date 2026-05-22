// Command api-gateway is the public REST entry point to AegisVision.
//
// It terminates client HTTP traffic at the edge of the cluster, enforces the
// platform's golden-path conventions (request id, RFC 9457 errors, OPA-based
// authorization, idempotency, cursor pagination, RED metrics, OTel tracing),
// and proxies to control-plane services over gRPC. The data plane is
// reachable through this gateway only via the realtime-hub, which is wired
// in a later phase.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aegisvision/pkg/platform/config"
	"github.com/aegisvision/pkg/platform/health"
	"github.com/aegisvision/pkg/platform/idempotency"
	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/middleware"
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/pagination"
	"github.com/aegisvision/pkg/platform/shutdown"

	"github.com/aegisvision/services/api-gateway/internal/auth"
	"github.com/aegisvision/services/api-gateway/internal/handlers"
	"github.com/aegisvision/services/api-gateway/internal/server"
	"github.com/aegisvision/services/api-gateway/internal/upstream"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "api-gateway"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8080")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8081")
	pipelineAddr := config.String("AEGIS_PIPELINE_SERVICE_ADDR", "localhost:9090")
	streamAddr := config.String("AEGIS_STREAM_MANAGER_ADDR", "")
	eventSvcURL := config.String("AEGIS_EVENT_SERVICE_URL", "")
	consoleDir := config.String("AEGIS_CONSOLE_DIR", "")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")
	opaEndpoint := config.String("AEGIS_OPA_ENDPOINT", "")
	cursorKey := config.String("AEGIS_CURSOR_KEY", "dev-cursor-key-change-me")
	jwksURL := config.String("AEGIS_JWKS_URL", "")
	jwtIssuer := config.String("AEGIS_JWT_ISSUER", "")
	jwtAudience := config.String("AEGIS_JWT_AUDIENCE", "")

	// Production-mode hardening: the api-gateway is the public face of the
	// platform; the cost of an insecure default in prod is the entire
	// platform. Refuse to start with any dev-only knob still set.
	if env != "dev" && env != "test" {
		if opaEndpoint == "" {
			return fmt.Errorf("AEGIS_OPA_ENDPOINT is required in env=%q (no AllowAll outside dev)", env)
		}
		if jwksURL == "" {
			return fmt.Errorf("AEGIS_JWKS_URL is required in env=%q", env)
		}
		if jwtIssuer == "" {
			return fmt.Errorf("AEGIS_JWT_ISSUER is required in env=%q", env)
		}
		if cursorKey == "dev-cursor-key-change-me" {
			return fmt.Errorf("AEGIS_CURSOR_KEY must be set to a non-default value in env=%q", env)
		}
		if len(cursorKey) < 32 {
			return fmt.Errorf("AEGIS_CURSOR_KEY must be at least 32 bytes in env=%q", env)
		}
	}

	logger := logging.Install(logging.Options{
		Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev",
	})
	logger.Info("starting",
		slog.String("version", version),
		slog.String("http_addr", httpAddr),
		slog.String("pipeline_service", pipelineAddr),
	)

	ctx := context.Background()
	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{
		ServiceName:    serviceName,
		ServiceVersion: version,
		Env:            env,
		OTLPEndpoint:   otlpEndpoint,
		Insecure:       env == "dev",
		SamplingRatio:  1.0,
	})
	if err != nil {
		return err
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)

	pipelineClient, err := upstream.Dial(pipelineAddr)
	if err != nil {
		return err
	}

	var authz middleware.AuthZDecider
	if opaEndpoint != "" {
		authz = auth.NewOPAClient(opaEndpoint)
		logger.Info("authz: OPA sidecar", slog.String("endpoint", opaEndpoint))
	} else {
		authz = auth.AllowAll(env)
		logger.Warn("authz: AllowAll — only valid in dev/test", slog.String("env", env))
	}

	// Authentication: when JWKS is configured, the gateway requires a
	// bearer token before any other middleware. In dev mode without JWKS
	// the gateway falls back to X-Tenant-Id (TenantFromHeader middleware).
	var authn func(http.Handler) http.Handler
	if jwksURL != "" {
		var err error
		authn, err = middleware.Authn(middleware.AuthnConfig{
			JWKSURL:  jwksURL,
			Issuer:   jwtIssuer,
			Audience: jwtAudience,
		})
		if err != nil {
			return err
		}
		logger.Info("authn: JWT/JWKS enabled", slog.String("issuer", jwtIssuer))
	} else {
		if env != "dev" && env != "test" {
			return fmt.Errorf("AEGIS_JWKS_URL must be configured in env=%q", env)
		}
		logger.Warn("authn: bearer token disabled (dev mode)")
	}

	var streamHandlers *handlers.StreamHandlers
	var streamClient *upstream.StreamClient
	if streamAddr != "" {
		streamClient, err = upstream.DialStream(streamAddr)
		if err != nil {
			return err
		}
		streamHandlers = &handlers.StreamHandlers{
			Streams: streamClient,
			Cursors: pagination.NewEncoder(cursorKey),
		}
	}

	var eventStream *handlers.EventStreamProxy
	if eventSvcURL != "" {
		eventStream = handlers.NewEventStreamProxy(eventSvcURL)
	}

	router := server.NewRouter(server.Deps{
		Logger:           logger,
		Metrics:          red,
		AuthZ:            authz,
		Authn:            authn,
		IdempotencyStore: idempotency.NewMemoryStore(0),
		IdempotencyTTL:   24 * time.Hour,
		Pipelines: &handlers.PipelineHandlers{
			Pipelines: pipelineClient,
			Cursors:   pagination.NewEncoder(cursorKey),
		},
		Streams:     streamHandlers,
		EventStream: eventStream,
		ConsoleDir:  consoleDir,
	})

	httpSrv := &http.Server{
		Addr:              httpAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Health + metrics surface lives on a separate listener so a runaway
	// workload on the public port can't starve probes/scrapes.
	reg2 := health.NewRegistry()
	reg2.Register("pipeline-service", func(ctx context.Context) error {
		// Cheap dial check — a real readiness should grpc.Health.Check.
		return nil
	})
	healthMux := http.NewServeMux()
	healthMux.Handle("/healthz", health.LivenessHandler())
	healthMux.Handle("/readyz", reg2.ReadinessHandler())
	healthMux.Handle("/metrics", metrics.Handler(reg))
	healthSrv := &http.Server{
		Addr:              healthAddr,
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	runner := shutdown.New(logger)
	runner.Register("http", httpSrv.Shutdown)
	runner.Register("health-http", healthSrv.Shutdown)
	runner.Register("pipeline-client", func(context.Context) error { return pipelineClient.Close() })
	if streamClient != nil {
		runner.Register("stream-client", func(context.Context) error { return streamClient.Close() })
	}
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http stopped", slog.String("err", err.Error()))
		}
	}()
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("health http stopped", slog.String("err", err.Error()))
		}
	}()
	return runner.Wait(ctx)
}
