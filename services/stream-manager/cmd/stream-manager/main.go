// Command stream-manager owns Stream resources and dispatches them onto
// dataplane runners via the operator.control NATS subject.
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
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/shutdown"

	"github.com/aegisvision/services/stream-manager/internal/server"
	"github.com/aegisvision/services/stream-manager/internal/service"
	"github.com/aegisvision/services/stream-manager/internal/store"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "stream-manager"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	grpcAddr := config.String("AEGIS_GRPC_ADDR", ":9092")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":9093")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	natsURL := config.String("AEGIS_NATS_URL", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	logger.Info("starting", slog.String("grpc_addr", grpcAddr), slog.String("version", version))

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
	_ = metrics.New(serviceName, reg)

	var publisher bus.Publisher
	if natsURL != "" {
		nc, err := bus.Connect(natsURL)
		if err != nil {
			return err
		}
		publisher = nc
		logger.Info("nats connected", slog.String("url", natsURL))
	} else {
		logger.Warn("AEGIS_NATS_URL unset — dispatch will be silent (control-plane-only mode)")
	}

	memstore := store.NewMemoryStore()
	svc := service.New(memstore, publisher, logger)

	grpcSrv, err := server.New(grpcAddr, svc)
	if err != nil {
		return err
	}

	healthReg := health.NewRegistry()
	healthReg.Register("store", func(context.Context) error { return nil })
	if publisher != nil {
		healthReg.Register("bus", func(context.Context) error { return nil })
	}

	mux := http.NewServeMux()
	mux.Handle("/healthz", health.LivenessHandler())
	mux.Handle("/readyz", healthReg.ReadinessHandler())
	mux.Handle("/metrics", metrics.Handler(reg))
	healthSrv := &http.Server{Addr: healthAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	runner := shutdown.New(logger)
	runner.Register("grpc", grpcSrv.Shutdown)
	runner.Register("health-http", healthSrv.Shutdown)
	if publisher != nil {
		runner.Register("bus", func(context.Context) error { return publisher.Close() })
	}
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
	return runner.Wait(ctx)
}
