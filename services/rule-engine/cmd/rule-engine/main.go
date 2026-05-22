// Command rule-engine subscribes to detections.v1, evaluates configured
// rules, and publishes resulting events.v1 messages. Rules are loaded from
// a YAML/JSON file at startup; a hot-reload signal handler reloads them on
// SIGHUP.
//
// HTTP diagnostic surface (/v1/rules, /v1/rules:eval, /v1/rules:reload)
// lives in internal/server. The bus loop is below — it's the load-bearing
// runtime concern of this service.
package main

import (
	"context"
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

	"github.com/aegisvision/services/rule-engine/internal/engine"
	"github.com/aegisvision/services/rule-engine/internal/server"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"
)

const serviceName = "rule-engine"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8210")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8211")
	natsURL := config.String("AEGIS_NATS_URL", "")
	rulesPath := config.String("AEGIS_RULES_PATH", "/etc/aegis/rules.json")
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
	fired := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aegis_rules_fired_total", Help: "Rule fires by rule_id",
	}, []string{"rule"})
	reg.MustRegister(fired)

	rules, err := loadRules(rulesPath)
	if err != nil {
		logger.Warn("no rules loaded", slog.String("err", err.Error()))
	}
	state := server.NewState(engine.New(rules), rules)

	nc, err := bus.Connect(natsURL)
	if err != nil {
		return err
	}
	sub, err := nc.Subscribe(ctx, "detections.v1", "rule-engine", func(ctx context.Context, m bus.Message) error {
		var d dataplanev1.Detection
		if err := proto.Unmarshal(m.Data, &d); err != nil {
			return bus.ErrSkip
		}
		for _, ev := range state.Engine().Apply(&d) {
			body, _ := proto.Marshal(ev)
			fired.WithLabelValues(ev.GetRuleId()).Inc()
			_ = nc.Publish(ctx, bus.Message{
				Subject: "events.v1", Data: body,
				Headers: map[string]string{
					"tenant": ev.GetTenantId(),
					"stream": ev.GetStreamId(),
					"rule":   ev.GetRuleId(),
				},
			})
		}
		return nil
	})
	if err != nil {
		return err
	}

	reload := func() error {
		newRules, lerr := loadRules(rulesPath)
		if lerr != nil {
			return lerr
		}
		state.Swap(engine.New(newRules), newRules)
		logger.Info("rules reloaded", slog.Int("count", len(newRules)))
		return nil
	}

	apiHandler := middleware.Recovery(logger)(
		middleware.RequestID()(
			middleware.Logging(logger)(
				red.HTTPMiddleware(metrics.RouteFromPattern)(
					middleware.TenantFromHeader()(server.Mux(state, reload))))))

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
	logger.Info("started", slog.Int("rules", len(rules)), slog.String("addr", httpAddr))
	return runner.Wait(ctx)
}

// Wire-friendly JSON form of a rule. Production deployments mount it via a
// ConfigMap or pull it from pipeline-service.
type wireRule struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	PipelineID string `json:"pipeline_id"`
	Kind       string `json:"kind"`
	Severity   int    `json:"severity"`
	Zone *struct {
		ID string  `json:"id"`
		X  float32 `json:"x"`
		Y  float32 `json:"y"`
		W  float32 `json:"w"`
		H  float32 `json:"h"`
	} `json:"zone,omitempty"`
	DwellMS  int64 `json:"dwell_ms,omitempty"`
	StaleMS  int64 `json:"stale_ms,omitempty"`
	Cooldown int64 `json:"cooldown_ms,omitempty"`
}

func loadRules(path string) ([]*engine.Rule, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wr []wireRule
	if err := json.Unmarshal(body, &wr); err != nil {
		return nil, err
	}
	out := make([]*engine.Rule, 0, len(wr))
	for _, r := range wr {
		rl := &engine.Rule{
			ID: r.ID, TenantID: r.TenantID, PipelineID: r.PipelineID,
			Kind:     engine.RuleKind(r.Kind),
			Dwell:    time.Duration(r.DwellMS) * time.Millisecond,
			Stale:    time.Duration(r.StaleMS) * time.Millisecond,
			Cooldown: time.Duration(r.Cooldown) * time.Millisecond,
		}
		if r.Zone != nil {
			rl.Zone = &engine.Zone{ID: r.Zone.ID, X: r.Zone.X, Y: r.Zone.Y, W: r.Zone.W, H: r.Zone.H}
		}
		out = append(out, rl)
	}
	return out, nil
}
