// Command dataplane-runner hosts the operator DAG for one or more pipelines.
// The walking-skeleton configuration ships a single replica that watches
// every pipeline via operator.control.> — production deployments partition
// pipelines across replicas using the queue-group consumer pattern (one
// delivery per group).
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

	"github.com/aegisvision/services/dataplane-runner/internal/runner"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "dataplane-runner"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":9095")
	natsURL := config.String("AEGIS_NATS_URL", "")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	if natsURL == "" {
		return errMissing("AEGIS_NATS_URL")
	}

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	logger.Info("starting", slog.String("nats", natsURL), slog.String("version", version))

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

	nc, err := bus.Connect(natsURL)
	if err != nil {
		return err
	}

	r := runner.NewRunner(nc, nc, logger, reg)
	if err := r.WatchAll(ctx); err != nil {
		return err
	}

	healthReg := health.NewRegistry()
	healthReg.Register("bus", func(context.Context) error { return nil })

	mux := http.NewServeMux()
	mux.Handle("/healthz", health.LivenessHandler())
	mux.Handle("/readyz", healthReg.ReadinessHandler())
	mux.Handle("/metrics", metrics.Handler(reg))
	healthSrv := &http.Server{Addr: healthAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	runnerShutdown := shutdown.New(logger)
	runnerShutdown.Register("runner", func(context.Context) error { return r.Close() })
	runnerShutdown.Register("health-http", healthSrv.Shutdown)
	runnerShutdown.Register("nats", func(context.Context) error { return nc.Close() })
	runnerShutdown.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })

	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("health http stopped", slog.String("err", err.Error()))
		}
	}()
	return runnerShutdown.Wait(ctx)
}

type missingErr string

func (m missingErr) Error() string { return string(m) }
func errMissing(name string) error { return missingErr("missing required env var: " + name) }
