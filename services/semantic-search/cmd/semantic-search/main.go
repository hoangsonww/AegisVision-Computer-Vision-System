// semantic-search: HTTP API for vector ingest + KNN query. The reference
// implementation is in-memory; production wires Qdrant behind the same API
// surface.
//
// HTTP routes live in internal/server; index logic in internal/index.
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

	"github.com/aegisvision/services/semantic-search/internal/index"
	"github.com/aegisvision/services/semantic-search/internal/server"
	"github.com/aegisvision/services/semantic-search/internal/service"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "semantic-search"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8290")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8291")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	ctx := context.Background()
	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{ServiceName: serviceName, ServiceVersion: version, Env: env, OTLPEndpoint: otlpEndpoint, Insecure: env == "dev", SamplingRatio: 1})
	if err != nil {
		return err
	}

	svc := service.New(index.New())

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	indexSize := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "aegis_semantic_index_size", Help: "Number of vectors in the index.",
	})
	reg.MustRegister(indexSize)

	apiHandler := middleware.Recovery(logger)(
		middleware.RequestID()(
			middleware.Logging(logger)(
				red.HTTPMiddleware(metrics.RouteFromPattern)(
					middleware.TenantFromHeader()(server.Mux(svc, server.Gauges{IndexSize: indexSize}))))))

	httpSrv := &http.Server{Addr: httpAddr, Handler: apiHandler, ReadHeaderTimeout: 5 * time.Second}
	hr := health.NewRegistry()
	hr.Register("index", func(context.Context) error { return nil })
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
