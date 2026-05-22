// Command audit-service consumes audit.v1 from the bus and exposes a small
// HTTP query API for compliance dashboards. Per ADR-0014 the audit log is
// the proof artifact for compliance reviews — append-only, low-trust input
// from many services, durable, queryable.
//
// HTTP routes live in internal/server; the bus consumer (with PK-conflict
// dedupe) lives in internal/consumer.
package main

import (
	"context"
	"errors"
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
	"github.com/aegisvision/pkg/store/sqldb"

	"github.com/aegisvision/services/audit-service/internal/consumer"
	"github.com/aegisvision/services/audit-service/internal/server"
	"github.com/aegisvision/services/audit-service/internal/store"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "audit-service"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8092")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8093")
	natsURL := config.String("AEGIS_NATS_URL", "")
	dbDriver := config.String("AEGIS_DB_DRIVER", "sqlite")
	dbDSN := config.String("AEGIS_DB_DSN", "file:/tmp/audit.sqlite?_pragma=journal_mode(WAL)")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	if natsURL == "" {
		return errors.New("AEGIS_NATS_URL is required")
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

	db, err := sqldb.Open(ctx, sqldb.Config{Driver: dbDriver, DSN: dbDSN})
	if err != nil {
		return err
	}
	if err := store.MigrateUp(ctx, db); err != nil {
		return err
	}
	s := store.New(db, dbDriver == sqldb.DriverPostgres)

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)

	nc, err := bus.Connect(natsURL)
	if err != nil {
		return err
	}
	con := consumer.New(logger, nc, s)
	if err := con.Start(ctx); err != nil {
		_ = nc.Close()
		return err
	}

	apiHandler := middleware.Recovery(logger)(
		middleware.RequestID()(
			middleware.Logging(logger)(
				red.HTTPMiddleware(metrics.RouteFromPattern)(
					middleware.TenantFromHeader()(server.Mux(s))))))

	httpSrv := &http.Server{Addr: httpAddr, Handler: apiHandler, ReadHeaderTimeout: 5 * time.Second}

	hr := health.NewRegistry()
	hr.Register("db", func(ctx context.Context) error { return db.PingContext(ctx) })

	healthMux := http.NewServeMux()
	healthMux.Handle("/healthz", health.LivenessHandler())
	healthMux.Handle("/readyz", hr.ReadinessHandler())
	healthMux.Handle("/metrics", metrics.Handler(reg))
	healthSrv := &http.Server{Addr: healthAddr, Handler: healthMux, ReadHeaderTimeout: 5 * time.Second}

	// LIFO: registered in reverse of execution order so the audit consumer
	// drains before the underlying NATS connection closes.
	runner := shutdown.New(logger)
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })
	runner.Register("db", func(context.Context) error { return db.Close() })
	runner.Register("nats", func(context.Context) error { return nc.Close() })
	runner.Register("consumer", func(context.Context) error { return con.Close() })
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
	logger.Info("started", slog.String("nats", natsURL))
	return runner.Wait(ctx)
}
