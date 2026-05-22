// Command model-registry is the control-plane catalog of Models and
// ModelVersions. STABLE promotion is gated on a verified CostProfile and,
// for face/person models, a fairness evaluation URI.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aegisvision/pkg/platform/config"
	"github.com/aegisvision/pkg/platform/health"
	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/shutdown"
	"github.com/aegisvision/pkg/store/sqldb"

	"github.com/aegisvision/services/model-registry/internal/server"
	"github.com/aegisvision/services/model-registry/internal/service"
	"github.com/aegisvision/services/model-registry/internal/store"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "model-registry"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	grpcAddr := config.String("AEGIS_GRPC_ADDR", ":9094")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":9097")
	dbDriver := config.String("AEGIS_DB_DRIVER", "sqlite")
	dbDSN := config.String("AEGIS_DB_DSN", "file:/tmp/model-registry.sqlite?_pragma=journal_mode(WAL)")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	if env != "dev" && env != "test" {
		if dbDriver != sqldb.DriverPostgres {
			slog.Warn("model-registry running with non-Postgres DB outside dev",
				slog.String("driver", dbDriver), slog.String("env", env))
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

	db, err := sqldb.Open(ctx, sqldb.Config{Driver: dbDriver, DSN: dbDSN})
	if err != nil {
		return err
	}
	if err := store.MigrateUp(ctx, db); err != nil {
		return err
	}
	logger.Info("db ready", slog.String("driver", dbDriver))

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	_ = metrics.New(serviceName, reg)

	svc := service.New(store.New(db, dbDriver == sqldb.DriverPostgres), logger)
	grpcSrv, err := server.New(grpcAddr, svc)
	if err != nil {
		return err
	}

	hr := health.NewRegistry()
	hr.Register("db", func(ctx context.Context) error { return db.PingContext(ctx) })

	mux := http.NewServeMux()
	mux.Handle("/healthz", health.LivenessHandler())
	mux.Handle("/readyz", hr.ReadinessHandler())
	mux.Handle("/metrics", metrics.Handler(reg))
	healthSrv := &http.Server{Addr: healthAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	runner := shutdown.New(logger)
	runner.Register("grpc", grpcSrv.Shutdown)
	runner.Register("health-http", healthSrv.Shutdown)
	runner.Register("db", func(context.Context) error { return db.Close() })
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })

	go func() {
		if err := grpcSrv.Serve(); err != nil {
			logger.Error("grpc stopped", slog.String("err", err.Error()))
		}
	}()
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("health http stopped", slog.String("err", err.Error()))
		}
	}()
	logger.Info("started", slog.String("addr", grpcSrv.Addr()), slog.String("env", env))
	return runner.Wait(ctx)
}
