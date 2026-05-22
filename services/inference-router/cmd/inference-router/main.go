// Command inference-router exposes an HTTP boundary in front of one or
// more Triton backends. Operator pods POST a frame payload + model
// selector and receive a JSON detection list.
//
// On every successful inference the service publishes two bus events:
//
//   - `inference.completed.v1` (metering-service bills on it)
//   - `inference.baseline.v1`  (shadow-inference-service consumes for
//                               canary regression detection)
//
// HTTP routes live in internal/server; the detector + bus publishing
// in internal/service.
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

	"github.com/aegisvision/services/inference-router/internal/server"
	"github.com/aegisvision/services/inference-router/internal/service"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "inference-router"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8094")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8095")
	tritonBase := config.String("AEGIS_TRITON_URL", "")
	natsURL := config.String("AEGIS_NATS_URL", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	ctx := context.Background()

	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{
		ServiceName: serviceName, ServiceVersion: version, Env: env,
		Insecure: env == "dev", SamplingRatio: 1.0,
	})
	if err != nil {
		return err
	}

	// Bus publisher — optional. In dev (no NATS) we run without one;
	// metering + shadow inference simply observe no events from this
	// router instance. Production must wire NATS.
	var publisher bus.Publisher
	if natsURL != "" {
		nc, err := bus.Connect(natsURL)
		if err != nil {
			return err
		}
		publisher = nc
	} else {
		logger.Warn("AEGIS_NATS_URL empty — no inference.* events will be published (dev only)")
	}

	svc := service.New(tritonBase, publisher)

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	infCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aegis_inference_invocations_total",
		Help: "Total inference invocations by model + outcome.",
	}, []string{"model", "version", "outcome"})
	reg.MustRegister(infCounter)

	apiHandler := middleware.Recovery(logger)(
		middleware.RequestID()(
			middleware.Logging(logger)(
				red.HTTPMiddleware(metrics.RouteFromPattern)(
					middleware.TenantFromHeader()(server.Mux(svc, server.Counters{Invocations: infCounter}))))))

	httpSrv := &http.Server{Addr: httpAddr, Handler: apiHandler, ReadHeaderTimeout: 5 * time.Second}

	hr := health.NewRegistry()
	hr.Register("self", func(context.Context) error { return nil })

	healthMux := http.NewServeMux()
	healthMux.Handle("/healthz", health.LivenessHandler())
	healthMux.Handle("/readyz", hr.ReadinessHandler())
	healthMux.Handle("/metrics", metrics.Handler(reg))
	healthSrv := &http.Server{Addr: healthAddr, Handler: healthMux, ReadHeaderTimeout: 5 * time.Second}

	runner := shutdown.New(logger)
	runner.Register("http", httpSrv.Shutdown)
	runner.Register("health-http", healthSrv.Shutdown)
	if publisher != nil {
		runner.Register("nats", func(context.Context) error { return publisher.Close() })
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
	logger.Info("started",
		slog.String("addr", httpAddr),
		slog.String("env", env),
		slog.Bool("triton_wired", tritonBase != ""),
		slog.Bool("bus_wired", publisher != nil),
	)
	return runner.Wait(ctx)
}
