// Command realtime-hub is the platform's WebSocket fanout for events. It
// subscribes to events.v1 from NATS and replays them to authenticated
// WebSocket (and SSE-fallback) subscribers. Tenant isolation is enforced
// at subscribe-time.
//
// HTTP transports live in internal/server. The bus loop is below.
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

	"github.com/aegisvision/services/realtime-hub/internal/hub"
	"github.com/aegisvision/services/realtime-hub/internal/server"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"
)

const serviceName = "realtime-hub"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8220")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8221")
	natsURL := config.String("AEGIS_NATS_URL", "")
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

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	subsGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "aegis_realtime_subscribers", Help: "Active WebSocket subscribers.",
	})
	droppedCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "aegis_realtime_dropped_total", Help: "Events dropped due to slow consumers.",
	})
	reg.MustRegister(subsGauge, droppedCounter)

	h := hub.New()

	nc, err := bus.Connect(natsURL)
	if err != nil {
		return err
	}
	sub, err := nc.Subscribe(ctx, "events.v1", "realtime-hub", func(ctx context.Context, m bus.Message) error {
		var ev dataplanev1.Event
		if err := proto.Unmarshal(m.Data, &ev); err != nil {
			return bus.ErrSkip
		}
		h.Publish(ctx, &ev)
		droppedCounter.Add(float64(h.Dropped()))
		return nil
	})
	if err != nil {
		return err
	}

	apiHandler := middleware.Recovery(logger)(
		middleware.RequestID()(
			middleware.Logging(logger)(
				red.HTTPMiddleware(metrics.RouteFromPattern)(
					middleware.TenantFromHeader()(server.Mux(h, server.Counters{
						Subscribers: subsGauge,
						Dropped:     droppedCounter,
					}))))))

	httpSrv := &http.Server{Addr: httpAddr, Handler: apiHandler, ReadHeaderTimeout: 5 * time.Second}

	hr := health.NewRegistry()
	hr.Register("bus", func(context.Context) error { return nil })
	hmux := http.NewServeMux()
	hmux.Handle("/healthz", health.LivenessHandler())
	hmux.Handle("/readyz", hr.ReadinessHandler())
	hmux.Handle("/metrics", metrics.Handler(reg))
	hsrv := &http.Server{Addr: healthAddr, Handler: hmux, ReadHeaderTimeout: 5 * time.Second}

	// LIFO: registered in reverse of execution order so subscriptions drain
	// before the underlying NATS connection closes.
	runner := shutdown.New(logger)
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })
	runner.Register("nats", func(context.Context) error { return nc.Close() })
	runner.Register("subscription", func(context.Context) error { return sub.Drain() })
	runner.Register("health-http", hsrv.Shutdown)
	runner.Register("http", httpSrv.Shutdown)

	go func() { _ = httpSrv.ListenAndServe() }()
	go func() { _ = hsrv.ListenAndServe() }()
	logger.Info("started", slog.String("addr", httpAddr))
	return runner.Wait(ctx)
}
