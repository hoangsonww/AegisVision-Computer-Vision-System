// Command pipeline-service is the control-plane gRPC service that owns the
// declarative pipeline definitions and their lifecycle state.
//
// Configuration is read from environment variables (the golden path); see
// the AEGIS_* env vars below for the surface.
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

	"github.com/aegisvision/services/pipeline-service/internal/server"
	"github.com/aegisvision/services/pipeline-service/internal/service"
	"github.com/aegisvision/services/pipeline-service/internal/store"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "pipeline-service"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	grpcAddr := config.String("AEGIS_GRPC_ADDR", ":9090")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":9091")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")
	seedDemo := config.Bool("AEGIS_SEED_DEMO", env == "dev")

	logger := logging.Install(logging.Options{
		Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev",
	})
	logger.Info("starting",
		slog.String("version", version),
		slog.String("grpc_addr", grpcAddr),
		slog.String("health_addr", healthAddr),
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
	_ = metrics.New(serviceName, reg) // declare metrics; gRPC interceptor wiring lives in the chart

	memstore := store.NewMemoryStore()
	if seedDemo {
		id := memstore.Seed("t-demo", "proj-demo", "Walking Skeleton")
		logger.Info("seeded demo pipeline", slog.String("pipeline_id", id))
	}
	temporalFrontend := config.String("AEGIS_TEMPORAL_FRONTEND", "")
	temporalNS := config.String("AEGIS_TEMPORAL_NS", "aegisvision")
	temporalTQ := config.String("AEGIS_TEMPORAL_TASKQUEUE", "pipeline-deploy")
	temporalWF := config.String("AEGIS_TEMPORAL_WORKFLOW", "DeployPipelineWorkflow")
	deployer := service.SelectDeployer(temporalFrontend, temporalNS, temporalTQ, temporalWF)
	if temporalFrontend != "" {
		logger.Info("temporal deployer wired", slog.String("frontend", temporalFrontend))
	}
	svc := service.New(memstore, deployer, logger)

	grpcSrv, err := server.New(grpcAddr, svc)
	if err != nil {
		return err
	}

	healthReg := health.NewRegistry()
	healthReg.Register("store", func(context.Context) error { return nil })

	healthMux := http.NewServeMux()
	healthMux.Handle("/healthz", health.LivenessHandler())
	healthMux.Handle("/readyz", healthReg.ReadinessHandler())
	healthMux.Handle("/metrics", metrics.Handler(reg))
	healthSrv := &http.Server{
		Addr:              healthAddr,
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	runner := shutdown.New(logger)
	runner.Register("grpc", grpcSrv.Shutdown)
	runner.Register("health-http", healthSrv.Shutdown)
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })

	go func() {
		if err := grpcSrv.Serve(); err != nil {
			logger.Error("grpc server stopped", slog.String("err", err.Error()))
		}
	}()
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("health server stopped", slog.String("err", err.Error()))
		}
	}()

	return runner.Wait(ctx)
}
