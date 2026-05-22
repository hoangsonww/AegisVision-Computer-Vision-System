// Command edge-gateway is the AegisVision edge-tier proxy. It runs at the
// camera site (typically inside a k3s cluster — ADR-0005), collects
// detections/events/telemetry from the local data plane, and forwards them
// to core over the WAN. When the WAN is down, items go to disk via the
// queue package; when the link returns, the gateway drains the queue in
// order.
//
// HTTP routes live in internal/server; the durable queue lives in
// internal/queue. The background forwarder is below.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/pkg/platform/config"
	"github.com/aegisvision/pkg/platform/health"
	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/middleware"
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/shutdown"

	"github.com/aegisvision/services/edge-gateway/internal/queue"
	"github.com/aegisvision/services/edge-gateway/internal/server"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "edge-gateway"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8320")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8321")
	coreNATS := config.String("AEGIS_CORE_NATS_URL", "")
	defaultQueueDir := "/var/lib/aegis/edge-queue"
	if env == "dev" || env == "test" {
		// Production deploys mount a writable volume at /var/lib/aegis; in dev
		// the path isn't writable, so fall back to a temp directory.
		defaultQueueDir = filepath.Join(os.TempDir(), "aegis-edge-queue")
	}
	queueDir := config.String("AEGIS_QUEUE_DIR", defaultQueueDir)
	maxItems := config.Int("AEGIS_QUEUE_MAX_ITEMS", 1_000_000)
	maxBytes := int64(config.Int("AEGIS_QUEUE_MAX_BYTES", 5<<30))
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	logger := logging.Install(logging.Options{Service: serviceName, Env: env, Level: logLevel, Pretty: env == "dev"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	otelShutdown, err := otelinit.Setup(ctx, otelinit.Options{
		ServiceName: serviceName, ServiceVersion: version, Env: env,
		OTLPEndpoint: otlpEndpoint, Insecure: env == "dev", SamplingRatio: 1.0,
	})
	if err != nil {
		return err
	}

	q, err := queue.New(queueDir, maxItems, maxBytes)
	if err != nil {
		return err
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	enqueued := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_edge_enqueued_total", Help: "Messages enqueued."})
	forwarded := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_edge_forwarded_total", Help: "Messages forwarded to core."})
	depthGauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "aegis_edge_queue_depth", Help: "Current queue depth."})
	reg.MustRegister(enqueued, forwarded, depthGauge)

	fwd := &queueForwarder{
		queue:     q,
		enqueued:  enqueued,
		forwarded: forwarded,
		depth:     depthGauge,
		coreNATS:  coreNATS,
	}

	apiHandler := middleware.Recovery(logger)(
		middleware.RequestID()(
			middleware.Logging(logger)(
				red.HTTPMiddleware(metrics.RouteFromPattern)(
					middleware.TenantFromHeader()(server.Mux(fwd))))))

	httpSrv := &http.Server{Addr: httpAddr, Handler: apiHandler, ReadHeaderTimeout: 5 * time.Second}

	// Background forwarder ticks every 2s when a core URL is configured.
	var forwardCancel context.CancelFunc
	var forwardWG sync.WaitGroup
	if coreNATS != "" {
		fctx, fcancel := context.WithCancel(ctx)
		forwardCancel = fcancel
		forwardWG.Add(1)
		go func() {
			defer forwardWG.Done()
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-fctx.Done():
					return
				case <-t.C:
				}
				if _, err := fwd.FlushNow(fctx); err != nil && err != context.Canceled {
					logger.WarnContext(fctx, "drain partial", slog.String("err", err.Error()))
				}
			}
		}()
	} else {
		logger.Warn("AEGIS_CORE_NATS_URL unset — running in disconnected mode (queue only)")
	}

	hr := health.NewRegistry()
	hr.Register("queue", func(context.Context) error { return nil })
	hmux := http.NewServeMux()
	hmux.Handle("/healthz", health.LivenessHandler())
	hmux.Handle("/readyz", hr.ReadinessHandler())
	hmux.Handle("/metrics", metrics.Handler(reg))
	hsrv := &http.Server{Addr: healthAddr, Handler: hmux, ReadHeaderTimeout: 5 * time.Second}

	runner := shutdown.New(logger)
	runner.Register("http", httpSrv.Shutdown)
	runner.Register("health-http", hsrv.Shutdown)
	if forwardCancel != nil {
		runner.Register("forwarder", func(context.Context) error { forwardCancel(); forwardWG.Wait(); return nil })
	}
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })

	go func() { _ = httpSrv.ListenAndServe() }()
	go func() { _ = hsrv.ListenAndServe() }()
	logger.Info("started",
		slog.String("addr", httpAddr),
		slog.String("queue_dir", queueDir),
		slog.Bool("forwarding", coreNATS != ""))
	return runner.Wait(ctx)
}

// queueForwarder satisfies server.Forwarder by adapting the disk queue.
type queueForwarder struct {
	queue     *queue.Queue
	enqueued  prometheus.Counter
	forwarded prometheus.Counter
	depth     prometheus.Gauge
	coreNATS  string
}

func (f *queueForwarder) Forward(_ context.Context, subject string, payload []byte) (uint64, error) {
	seq, err := f.queue.Put(subject, payload)
	if err != nil {
		return 0, err
	}
	f.enqueued.Inc()
	f.depth.Set(float64(f.queue.Depth()))
	return seq, nil
}

func (f *queueForwarder) Stats() server.Stats {
	return server.Stats{Items: f.queue.Depth()}
}

// FlushNow attempts a single drain pass. If no upstream NATS URL is
// configured (disconnected mode) we return 0 with no error — the caller
// then knows nothing is moving.
func (f *queueForwarder) FlushNow(ctx context.Context) (int, error) {
	if f.coreNATS == "" {
		return 0, nil
	}
	nc, err := bus.Connect(f.coreNATS)
	if err != nil {
		return 0, err
	}
	defer nc.Close()
	count := 0
	derr := f.queue.Drain(ctx, func(c context.Context, it queue.Item) error {
		if err := nc.Publish(c, bus.Message{Subject: it.Subject, Data: it.Data}); err != nil {
			return err
		}
		count++
		f.forwarded.Inc()
		return nil
	})
	f.depth.Set(float64(f.queue.Depth()))
	if derr != nil && derr != context.Canceled {
		return count, derr
	}
	return count, nil
}
