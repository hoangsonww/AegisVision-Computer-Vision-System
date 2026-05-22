// Command gpu-scheduler exposes the platform's GPU reservation ledger over
// HTTP. Production deployments back the ledger with Redis; this binary
// ships the in-process implementation that is the unit-test target and
// the source-of-truth for the algorithm (ADR-0003).
//
// HTTP routes live in internal/server; placement logic in internal/service.
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
	"github.com/aegisvision/pkg/platform/middleware"
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/shutdown"

	"github.com/aegisvision/services/gpu-scheduler/internal/ledger"
	"github.com/aegisvision/services/gpu-scheduler/internal/server"
	"github.com/aegisvision/services/gpu-scheduler/internal/service"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "gpu-scheduler"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8200")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8201")
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

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	placeCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aegis_gpu_placement_total",
		Help: "Placement decisions by outcome.",
	}, []string{"outcome"})
	freeVRAMGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aegis_gpu_free_vram_bytes",
		Help: "Free VRAM bytes per GPU as the ledger sees it (NOT nvidia-smi).",
	}, []string{"gpu", "sku"})
	reg.MustRegister(placeCounter, freeVRAMGauge)

	svc := service.New(ledger.New())

	apiHandler := middleware.Recovery(logger)(
		middleware.RequestID()(
			middleware.Logging(logger)(
				red.HTTPMiddleware(metrics.RouteFromPattern)(
					middleware.TenantFromHeader()(server.Mux(svc, server.Counters{
						Placement: placeCounter,
						FreeVRAM:  freeVRAMGauge,
					}))))))

	httpSrv := &http.Server{Addr: httpAddr, Handler: apiHandler, ReadHeaderTimeout: 5 * time.Second}
	hr := health.NewRegistry()
	hr.Register("ledger", func(context.Context) error { return nil })

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
