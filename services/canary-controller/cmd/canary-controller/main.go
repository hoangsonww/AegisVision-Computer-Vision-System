// Command canary-controller is the Phase-5 progressive-delivery service
// (ADR-0023). It owns:
//
//   - CanaryPlan CRUD (REST)
//   - Observation ingestion from event-service / shadow-inference-service
//   - A reconciliation loop that advances or rolls back plans based on
//     the statistical regression detector in pkg/canary.
//
// Promotion is consequential and gated; rollback is reversible and
// auto-applied. The loop's outputs (canary.transition.v1,
// canary.rollback.v1, canary.promote.v1) are consumed by model-registry
// to actually swap routing weights, and by audit-service to record.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

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

	"github.com/aegisvision/pkg/canary"
	"github.com/aegisvision/services/canary-controller/internal/loop"
	"github.com/aegisvision/services/canary-controller/internal/store"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "canary-controller"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8460")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8461")
	dbDriver := config.String("AEGIS_DB_DRIVER", "sqlite")
	dbDSN := config.String("AEGIS_DB_DSN", "file:/tmp/canary.sqlite?_pragma=journal_mode(WAL)")
	natsURL := config.String("AEGIS_NATS_URL", "")
	tickInterval := config.Duration("AEGIS_CANARY_TICK", 15*time.Second)
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

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	plansActive := prometheus.NewGauge(prometheus.GaugeOpts{Name: "aegis_canary_plans_active", Help: "Active canary plans."})
	transitions := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_canary_transitions_total", Help: "Canary transitions."}, []string{"to"})
	obsIngested := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_canary_observations_ingested_total", Help: "Observations ingested."})
	reg.MustRegister(plansActive, transitions, obsIngested)

	rec := &loop.Reconciler{Store: st, Bus: pub, Logger: logger, Lookback: 5 * time.Minute}
	go runTicker(ctx, logger, tickInterval, st, rec, plansActive, transitions)

	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(
				middleware.TenantFromHeader()(h)))))
	}

	mux.Handle("POST /v1/canary/plans", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p intelligence.CanaryPlan
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		p.TenantID = logging.TenantID(r.Context())
		if p.ID == "" {
			p.ID = fmt.Sprintf("cp-%d", time.Now().UnixNano())
		}
		if p.Status == "" {
			p.Status = intelligence.CanaryStatusRamping
		}
		if p.LastStepAt.IsZero() {
			p.LastStepAt = time.Now().UTC()
		}
		if err := st.CreatePlan(r.Context(), p); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, p)
	}), "/v1/canary/plans"))

	mux.Handle("GET /v1/canary/plans", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		status := intelligence.CanaryStatus(r.URL.Query().Get("status"))
		out, err := st.ListPlans(r.Context(), tenant, status)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}), "/v1/canary/plans"))

	mux.Handle("GET /v1/canary/plans/{id}", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := st.GetPlan(r.Context(), r.PathValue("id"))
		if err != nil {
			problem.NotFound("plan not found").Write(w)
			return
		}
		if tenant := logging.TenantID(r.Context()); tenant != "" && tenant != p.TenantID {
			problem.Forbidden("cross-tenant read denied").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}), "/v1/canary/plans/{id}"))

	mux.Handle("POST /v1/canary/observations", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var obs []intelligence.CanaryObservation
		if err := json.NewDecoder(r.Body).Decode(&obs); err != nil {
			problem.BadRequest("expect array of observations").Write(w)
			return
		}
		for _, o := range obs {
			if o.WindowEnd.IsZero() {
				o.WindowEnd = time.Now().UTC()
			}
			if o.WindowStart.IsZero() {
				o.WindowStart = o.WindowEnd.Add(-1 * time.Minute)
			}
			if err := st.IngestObservation(r.Context(), o); err == nil {
				obsIngested.Inc()
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}), "/v1/canary/observations"))

	// Manual operator controls — pause/resume are reversible (tier 2);
	// approve-promote / rollback are exposed but the caller MUST come via
	// policy-gate-service for promotion (the loop emits canary.promote.v1
	// only after the gate resolves).
	mux.Handle("POST /v1/canary/plans/{id}/pause", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = st.ApplyDecision(r.Context(), r.PathValue("id"), canary.Decision{
			NewStatus: intelligence.CanaryStatusPaused, NewStep: -1, HumanReadable: "operator pause",
		})
		writeJSON(w, http.StatusOK, map[string]any{"status": "paused"})
	}), "/v1/canary/plans/{id}/pause"))
	mux.Handle("POST /v1/canary/plans/{id}/resume", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = st.ApplyDecision(r.Context(), r.PathValue("id"), canary.Decision{
			NewStatus: intelligence.CanaryStatusRamping, NewStep: -1, HumanReadable: "operator resume",
		})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ramping"})
	}), "/v1/canary/plans/{id}/resume"))

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

func runTicker(ctx context.Context, logger *slog.Logger, every time.Duration, st *store.Store, rec *loop.Reconciler, plansActive prometheus.Gauge, transitions *prometheus.CounterVec) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			active, _ := st.ListActive(ctx)
			plansActive.Set(float64(len(active)))
			n, err := rec.Tick(ctx, time.Now().UTC())
			if err != nil {
				logger.Warn("tick error", slog.String("err", err.Error()))
				continue
			}
			if n > 0 {
				transitions.WithLabelValues("any").Add(float64(n))
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
