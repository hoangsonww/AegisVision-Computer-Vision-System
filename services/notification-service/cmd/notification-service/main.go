// Command notification-service consumes events.v1 from the bus and fans
// them out to configured webhook endpoints with HMAC signing + retries.
//
// HTTP routes (endpoint CRUD) live in internal/server. The delivery
// client lives in internal/delivery. The bus subscriber + DLQ wiring
// is below — it's the load-bearing runtime concern of this service.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

	"github.com/aegisvision/services/notification-service/internal/delivery"
	"github.com/aegisvision/services/notification-service/internal/server"
	"github.com/aegisvision/services/notification-service/internal/store"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"
)

const serviceName = "notification-service"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8230")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8231")
	natsURL := config.String("AEGIS_NATS_URL", "")
	dbDriver := config.String("AEGIS_DB_DRIVER", "sqlite")
	dbDSN := config.String("AEGIS_DB_DSN", "file:/tmp/notification.sqlite?_pragma=journal_mode(WAL)")
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
	st := store.New(db, dbDriver == sqldb.DriverPostgres)

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	delivered := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aegis_webhook_delivered_total", Help: "Webhook deliveries by outcome.",
	}, []string{"outcome"})
	reg.MustRegister(delivered)

	client := delivery.NewClient(5 * time.Second)
	nc, err := bus.Connect(natsURL)
	if err != nil {
		return err
	}
	sub, err := nc.Subscribe(ctx, "events.v1", "notification-service", func(ctx context.Context, m bus.Message) error {
		var ev dataplanev1.Event
		if err := proto.Unmarshal(m.Data, &ev); err != nil {
			return bus.ErrSkip
		}
		eps, err := st.ListByTenant(ctx, ev.GetTenantId())
		if err != nil || len(eps) == 0 {
			return nil
		}
		body, _ := json.Marshal(map[string]any{
			"id": ev.GetId(), "tenant_id": ev.GetTenantId(),
			"pipeline_id": ev.GetPipelineId(), "stream_id": ev.GetStreamId(),
			"kind": ev.GetKind().String(), "severity": ev.GetSeverity().String(),
			"summary": ev.GetSummary(), "rule_id": ev.GetRuleId(),
		})
		for _, ep := range eps {
			out := client.Deliver(ctx, delivery.Endpoint{
				URL: ep.URL, Secret: ep.Secret, Tenant: ep.TenantID,
			}, body)
			if out.Err != nil || out.StatusCode >= 300 {
				delivered.WithLabelValues("error").Inc()
				_ = st.RecordDLQ(ctx, store.DLQEntry{
					ID:         newID(),
					EndpointID: ep.ID, TenantID: ep.TenantID,
					Body:      body,
					Attempts:  out.Attempt,
					LastError: errMsg(out.Err, out.StatusCode),
					FailedAt:  time.Now().UTC(),
				})
				_ = nc.Publish(ctx, bus.Message{Subject: "notifications.dlq.v1", Data: body})
			} else {
				delivered.WithLabelValues("ok").Inc()
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	apiHandler := middleware.Recovery(logger)(
		middleware.RequestID()(
			middleware.Logging(logger)(
				red.HTTPMiddleware(metrics.RouteFromPattern)(
					middleware.TenantFromHeader()(server.Mux(st))))))

	httpSrv := &http.Server{Addr: httpAddr, Handler: apiHandler, ReadHeaderTimeout: 5 * time.Second}

	hr := health.NewRegistry()
	hr.Register("db", func(ctx context.Context) error { return db.PingContext(ctx) })
	hmux := http.NewServeMux()
	hmux.Handle("/healthz", health.LivenessHandler())
	hmux.Handle("/readyz", hr.ReadinessHandler())
	hmux.Handle("/metrics", metrics.Handler(reg))
	hsrv := &http.Server{Addr: healthAddr, Handler: hmux, ReadHeaderTimeout: 5 * time.Second}

	// LIFO: registered in reverse of execution order so subscriptions drain
	// before the underlying NATS connection closes.
	runner := shutdown.New(logger)
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })
	runner.Register("db", func(context.Context) error { return db.Close() })
	runner.Register("nats", func(context.Context) error { return nc.Close() })
	runner.Register("subscription", func(context.Context) error { return sub.Drain() })
	runner.Register("health-http", hsrv.Shutdown)
	runner.Register("http", httpSrv.Shutdown)

	go func() { _ = httpSrv.ListenAndServe() }()
	go func() { _ = hsrv.ListenAndServe() }()
	logger.Info("started", slog.String("addr", httpAddr))
	return runner.Wait(ctx)
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func errMsg(err error, status int) string {
	if err != nil {
		return err.Error()
	}
	return "HTTP " + http.StatusText(status)
}
