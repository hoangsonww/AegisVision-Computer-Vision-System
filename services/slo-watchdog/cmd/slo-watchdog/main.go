// Command slo-watchdog evaluates SLO targets every interval and emits
// burn-rate signals when budget consumption exceeds the multi-window
// thresholds from the Google SRE workbook (table 5-7).
//
// Each target's PromQL must return a 0..1 *success ratio* — the watchdog
// computes the burn rate from there using BurnRate(observed, target).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aegisvision/pkg/autonomy"
	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/platform/config"
	"github.com/aegisvision/pkg/platform/health"
	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/middleware"
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/problem"
	"github.com/aegisvision/pkg/platform/shutdown"
	"github.com/aegisvision/pkg/store/sqldb"

	"github.com/aegisvision/services/slo-watchdog/internal/prom"
	"github.com/aegisvision/services/slo-watchdog/internal/store"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "slo-watchdog"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8490")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8491")
	dbDriver := config.String("AEGIS_DB_DRIVER", "sqlite")
	dbDSN := config.String("AEGIS_DB_DSN", "file:/tmp/slo.sqlite?_pragma=journal_mode(WAL)")
	natsURL := config.String("AEGIS_NATS_URL", "")
	promURL := config.String("AEGIS_PROMETHEUS_URL", "")
	tickEvery := config.Duration("AEGIS_SLO_TICK", 60*time.Second)
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	if env != "dev" && env != "test" {
		if promURL == "" {
			return fmt.Errorf("AEGIS_PROMETHEUS_URL is required outside dev")
		}
	}

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
	db, err := sqldb.Open(ctx, sqldb.Config{Driver: dbDriver, DSN: dbDSN})
	if err != nil {
		return err
	}
	if err := store.MigrateUp(ctx, db); err != nil {
		return err
	}
	st := store.New(db, dbDriver == sqldb.DriverPostgres)

	var pub bus.Publisher
	if natsURL != "" {
		nc, err := bus.Connect(natsURL)
		if err != nil {
			return err
		}
		pub = nc
	} else {
		pub = bus.NewInMemory()
	}
	pq := prom.New(promURL)

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	burnGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "aegis_slo_burn_rate", Help: "Current burn rate per target/window."}, []string{"target", "window"})
	signals := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_slo_signals_total", Help: "SLO signals emitted."}, []string{"severity"})
	reg.MustRegister(burnGauge, signals)

	go func() {
		t := time.NewTicker(tickEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				tick(ctx, logger, st, pq, pub, burnGauge, signals, promURL != "")
			}
		}
	}()

	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(
				middleware.TenantFromHeader()(h)))))
	}

	mux.Handle("POST /v1/slo/targets", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var t intelligence.SLOTarget
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		t.TenantID = logging.TenantID(r.Context())
		if t.ID == "" {
			t.ID = fmt.Sprintf("slo-%d", time.Now().UnixNano())
		}
		if t.RollingWindowSeconds == 0 {
			t.RollingWindowSeconds = 28 * 24 * 3600
		}
		if t.Target == 0 {
			t.Target = 0.999
		}
		if err := st.UpsertTarget(r.Context(), t); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, t)
	}), "/v1/slo/targets"))

	mux.Handle("GET /v1/slo/targets", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, _ := st.ListByTenant(r.Context(), logging.TenantID(r.Context()))
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}), "/v1/slo/targets"))

	mux.Handle("GET /v1/slo/signals", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		since := time.Now().Add(-24 * time.Hour)
		if s := r.URL.Query().Get("since"); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				since = t
			}
		}
		out, _ := st.ListSignals(r.Context(), logging.TenantID(r.Context()), since, 100)
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}), "/v1/slo/signals"))

	httpSrv := &http.Server{Addr: httpAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	hr := health.NewRegistry()
	hr.Register("db", func(ctx context.Context) error { return db.PingContext(ctx) })
	hmux := http.NewServeMux()
	hmux.Handle("/healthz", health.LivenessHandler())
	hmux.Handle("/readyz", hr.ReadinessHandler())
	hmux.Handle("/metrics", metrics.Handler(reg))
	hsrv := &http.Server{Addr: healthAddr, Handler: hmux, ReadHeaderTimeout: 5 * time.Second}

	runner := shutdown.New(logger)
	runner.Register("http", httpSrv.Shutdown)
	runner.Register("health-http", hsrv.Shutdown)
	runner.Register("bus", func(context.Context) error { return pub.Close() })
	runner.Register("db", func(context.Context) error { return db.Close() })
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })

	go func() { _ = httpSrv.ListenAndServe() }()
	go func() { _ = hsrv.ListenAndServe() }()
	logger.Info("started", slog.String("addr", httpAddr))
	return runner.Wait(ctx)
}

func tick(ctx context.Context, logger *slog.Logger, st *store.Store, pq *prom.Client, pub bus.Publisher, burnGauge *prometheus.GaugeVec, signals *prometheus.CounterVec, havePromQL bool) {
	if !havePromQL {
		return
	}
	targets, err := st.ListActiveTargets(ctx)
	if err != nil {
		return
	}
	for _, t := range targets {
		now := time.Now().UTC()
		// We request three windows by appending Prometheus rate-window
		// modifiers — but to keep this service simple we just query the
		// raw PromQL and treat it as the *current* success ratio. Callers
		// should write PromQL that already aggregates over the desired
		// window (e.g. "avg_over_time(metric[1h])").
		obs1h, err1 := pq.Query(ctx, withWindow(t.PromQL, "1h"), now)
		obs6h, err6 := pq.Query(ctx, withWindow(t.PromQL, "6h"), now)
		obs24h, err24 := pq.Query(ctx, withWindow(t.PromQL, "24h"), now)
		if err1 != nil && err6 != nil && err24 != nil {
			logger.Warn("slo query failed", slog.String("target", t.ID), slog.String("err", err1.Error()))
			continue
		}
		br1 := autonomy.BurnRate(obs1h, t.Target)
		br6 := autonomy.BurnRate(obs6h, t.Target)
		br24 := autonomy.BurnRate(obs24h, t.Target)
		burnGauge.WithLabelValues(t.ID, "1h").Set(br1)
		burnGauge.WithLabelValues(t.ID, "6h").Set(br6)
		burnGauge.WithLabelValues(t.ID, "24h").Set(br24)
		severity := autonomy.MultiWindow(br1, br6, br24)
		if severity == "" {
			continue
		}
		sig := intelligence.SLOSignal{
			ID: fmt.Sprintf("ss-%d", time.Now().UnixNano()),
			TenantID: t.TenantID, TargetID: t.ID,
			BurnRate1h: br1, BurnRate6h: br6, BurnRate24h: br24,
			Severity: severity, EmittedAt: now,
		}
		_ = st.InsertSignal(ctx, sig)
		signals.WithLabelValues(severity).Inc()
		body, _ := json.Marshal(sig)
		_ = pub.Publish(ctx, bus.Message{
			Subject: "autonomy.signal.slo.v1", Data: body,
			Headers: map[string]string{"tenant_id": t.TenantID, "target_id": t.ID, "severity": severity},
		})
	}
}

// withWindow wraps a PromQL expression that produces a per-window success
// ratio. The expected user-provided shape is something like
//   "sum(rate(success[__WINDOW__])) / sum(rate(total[__WINDOW__]))"
// where __WINDOW__ is the placeholder we substitute. If the placeholder
// is missing, we leave the expression alone (treating it as already
// scoped to a window).
func withWindow(expr, window string) string {
	if !contains(expr, "__WINDOW__") {
		return expr
	}
	out := ""
	for i := 0; i < len(expr); {
		if i+len("__WINDOW__") <= len(expr) && expr[i:i+len("__WINDOW__")] == "__WINDOW__" {
			out += window
			i += len("__WINDOW__")
			continue
		}
		out += string(expr[i])
		i++
	}
	return out
}

func contains(s, sub string) bool {
	return len(sub) <= len(s) && (s == sub || index(s, sub) >= 0)
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
