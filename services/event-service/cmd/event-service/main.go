// Command event-service consumes events.v1 from the bus, retains a recent-
// events ring (placeholder for the ClickHouse-backed read store in
// production), and serves the realtime SSE feed the console subscribes to.
package main

import (
	"context"
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

	"github.com/aegisvision/services/event-service/internal/consumer"
	"github.com/aegisvision/services/event-service/internal/server"
	"github.com/aegisvision/services/event-service/internal/store"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "event-service"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8090")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8091")
	natsURL := config.String("AEGIS_NATS_URL", "")
	embedNATS := config.Bool("AEGIS_EMBED_NATS", env == "dev")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")
	ringCap := config.Int("AEGIS_EVENT_RING_CAP", 10000)

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	logger.Info("starting", slog.String("http_addr", httpAddr), slog.String("version", version))

	ctx := context.Background()
	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{
		ServiceName: serviceName, ServiceVersion: version, Env: env,
		OTLPEndpoint: otlpEndpoint, Insecure: env == "dev", SamplingRatio: 1.0,
	})
	if err != nil {
		return err
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)

	var stopNATS func()
	if natsURL == "" {
		if !embedNATS {
			return errMissing("AEGIS_NATS_URL")
		}
		url, srv, err := bus.EmbeddedServer()
		if err != nil {
			return err
		}
		natsURL = url
		stopNATS = func() { srv.Shutdown() }
		logger.Info("embedded NATS started", slog.String("url", natsURL))
	}

	nc, err := bus.Connect(natsURL)
	if err != nil {
		return err
	}

	ring := store.New(ringCap)
	subscription, err := consumer.Start(ctx, nc, ring, logger)
	if err != nil {
		return err
	}
	logger.Info("subscribed to events.v1")

	sse := &server.SSE{Store: ring}
	search := &server.Search{Store: ring}

	mux := http.NewServeMux()
	mux.Handle("/v1/events:stream", chain(
		middleware.Recovery(logger),
		middleware.RequestID(),
		middleware.Logging(logger),
		red.HTTPMiddleware(func(*http.Request) string { return "/v1/events:stream" }),
		middleware.TenantFromHeader(),
	)(sse.Handler()))
	mux.Handle("GET /v1/events", chain(
		middleware.Recovery(logger),
		middleware.RequestID(),
		middleware.Logging(logger),
		red.HTTPMiddleware(func(*http.Request) string { return "/v1/events" }),
		middleware.TenantFromHeader(),
	)(search.Handler()))

	httpSrv := &http.Server{Addr: httpAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	healthReg := health.NewRegistry()
	healthReg.Register("bus", func(context.Context) error { return nil })

	healthMux := http.NewServeMux()
	healthMux.Handle("/healthz", health.LivenessHandler())
	healthMux.Handle("/readyz", healthReg.ReadinessHandler())
	healthMux.Handle("/metrics", metrics.Handler(reg))
	healthSrv := &http.Server{Addr: healthAddr, Handler: healthMux, ReadHeaderTimeout: 5 * time.Second}

	// Shutdown closers run in LIFO order, so register them in reverse of the
	// intended execution order. We want: stop accepting requests → drain
	// subscriptions → close NATS conn → stop embedded NATS → flush OTel.
	runner := shutdown.New(logger)
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })
	if stopNATS != nil {
		runner.Register("embedded-nats", func(context.Context) error { stopNATS(); return nil })
	}
	runner.Register("nats", func(context.Context) error { return nc.Close() })
	runner.Register("subscription", func(context.Context) error { return subscription.Drain() })
	runner.Register("health-http", healthSrv.Shutdown)
	runner.Register("http", httpSrv.Shutdown)

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

func chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

type missingErr string

func (m missingErr) Error() string { return string(m) }
func errMissing(name string) error { return missingErr("missing required env var: " + name) }
