// Command drift-detection-service implements ADR-0025: continuous
// concept-drift monitoring per (tenant, model) using KL/JS/TVD divergence
// against a captured reference distribution.
//
// Inputs:
//
//   - detections.v1                 — primary detection firehose
//   - HTTP CRUD for policies + refs — operator-controlled
//
// Outputs:
//
//   - autonomy.signal.drift.v1     — bus event (consumed by autonomy-orchestrator)
//   - aegis_drift_* metrics        — for SRE dashboards
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
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

	"github.com/aegisvision/services/drift-detection-service/internal/store"
	"github.com/aegisvision/services/drift-detection-service/internal/window"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "drift-detection-service"

// detectionEvent mirrors the firehose event shape used by the active-
// learning service. Both services consume detections.v1.
type detectionEvent struct {
	TenantID       string `json:"tenant_id"`
	ModelID        string `json:"model_id"`
	PredictedClass string `json:"predicted_class"`
}

type rollKey struct {
	tenant, model string
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8480")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8481")
	dbDriver := config.String("AEGIS_DB_DRIVER", "sqlite")
	dbDSN := config.String("AEGIS_DB_DSN", "file:/tmp/drift.sqlite?_pragma=journal_mode(WAL)")
	natsURL := config.String("AEGIS_NATS_URL", "")
	evalEvery := config.Duration("AEGIS_DRIFT_EVAL", 30*time.Second)
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
	var sub bus.Subscriber
	if natsURL != "" {
		nc, err := bus.Connect(natsURL)
		if err != nil {
			return err
		}
		pub, sub = nc, nc
	} else {
		mem := bus.NewInMemory()
		pub, sub = mem, mem
	}

	// Per (tenant, model) rolling windows. Default 15-minute width;
	// policy overrides on demand via /v1/drift/policies.
	rolls := struct {
		sync.Mutex
		m map[rollKey]*window.Rolling
	}{m: map[rollKey]*window.Rolling{}}
	getRoll := func(t, m string, width int) *window.Rolling {
		rolls.Lock()
		defer rolls.Unlock()
		k := rollKey{tenant: t, model: m}
		r, ok := rolls.m[k]
		if !ok {
			r = window.New(width)
			rolls.m[k] = r
		}
		return r
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	observed := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_drift_observed_total", Help: "Detections observed."})
	signals := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_drift_signals_total", Help: "Drift signals emitted."}, []string{"severity"})
	divergence := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "aegis_drift_divergence", Help: "Latest divergence per tenant+model."}, []string{"tenant", "model"})
	reg.MustRegister(observed, signals, divergence)

	_, _ = sub.Subscribe(ctx, "detections.v1", "drift-detection", func(ctx context.Context, msg bus.Message) error {
		var ev detectionEvent
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			return nil
		}
		if ev.TenantID == "" || ev.ModelID == "" || ev.PredictedClass == "" {
			return nil
		}
		// Use the policy's configured window if known; otherwise default 900.
		ps, _ := st.ListPoliciesByModel(ctx, ev.TenantID, ev.ModelID)
		width := 900
		if len(ps) > 0 && ps[0].WindowSeconds > 0 {
			width = ps[0].WindowSeconds
		}
		getRoll(ev.TenantID, ev.ModelID, width).Observe(ev.PredictedClass)
		observed.Inc()
		return nil
	})

	// Periodic evaluation.
	go func() {
		t := time.NewTicker(evalEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				evalAll(ctx, logger, st, &rolls, pub, signals, divergence)
			}
		}
	}()

	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(
				middleware.TenantFromHeader()(h)))))
	}

	mux.Handle("POST /v1/drift/policies", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p intelligence.DriftPolicy
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		p.TenantID = logging.TenantID(r.Context())
		if p.ID == "" {
			p.ID = fmt.Sprintf("dp-%d", time.Now().UnixNano())
		}
		if p.Method == "" {
			p.Method = intelligence.DivergenceJS
		}
		if err := st.UpsertPolicy(r.Context(), p); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, p)
	}), "/v1/drift/policies"))

	mux.Handle("POST /v1/drift/references", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ref intelligence.DriftReference
		if err := json.NewDecoder(r.Body).Decode(&ref); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		ref.TenantID = logging.TenantID(r.Context())
		if ref.ID == "" {
			ref.ID = fmt.Sprintf("dr-%d", time.Now().UnixNano())
		}
		// Normalize classes.
		ref.Distribution.Classes = autonomy.Normalize(ref.Distribution.Classes)
		if err := st.UpsertReference(r.Context(), ref); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, ref)
	}), "/v1/drift/references"))

	mux.Handle("GET /v1/drift/signals", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		since := time.Now().Add(-24 * time.Hour)
		if s := r.URL.Query().Get("since"); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				since = t
			}
		}
		out, err := st.ListSignals(r.Context(), tenant, since, 200)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}), "/v1/drift/signals"))

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

func evalAll(ctx context.Context, logger *slog.Logger, st *store.Store,
	rolls *struct {
		sync.Mutex
		m map[rollKey]*window.Rolling
	},
	pub bus.Publisher,
	signals *prometheus.CounterVec, divergence *prometheus.GaugeVec) {
	rolls.Lock()
	snapshots := make(map[rollKey]intelligence.Distribution, len(rolls.m))
	for k, r := range rolls.m {
		snapshots[k] = r.Snapshot()
	}
	rolls.Unlock()
	for k, snap := range snapshots {
		ps, _ := st.ListPoliciesByModel(ctx, k.tenant, k.model)
		if len(ps) == 0 {
			continue
		}
		ref, err := st.GetReference(ctx, k.tenant, k.model)
		if err != nil || ref == nil || len(ref.Distribution.Classes) == 0 {
			continue
		}
		policy := ps[0]
		if snap.Samples < policy.MinSamples {
			continue
		}
		d := autonomy.Divergence(policy.Method, snap.Classes, ref.Distribution.Classes)
		divergence.WithLabelValues(k.tenant, k.model).Set(d)
		severity := "info"
		switch {
		case d >= policy.CriticalThreshold:
			severity = "critical"
		case d >= policy.WarnThreshold:
			severity = "warn"
		default:
			continue
		}
		sig := intelligence.DriftSignal{
			ID:         fmt.Sprintf("ds-%d", time.Now().UnixNano()),
			TenantID:   k.tenant, ModelID: k.model,
			Method:     policy.Method,
			Divergence: d, Severity: severity,
			Observed:   snap,
			EmittedAt:  time.Now().UTC(),
		}
		if err := st.InsertSignal(ctx, sig); err != nil {
			logger.Warn("insert drift signal failed", slog.String("err", err.Error()))
			continue
		}
		signals.WithLabelValues(severity).Inc()
		body, _ := json.Marshal(sig)
		_ = pub.Publish(ctx, bus.Message{
			Subject: "autonomy.signal.drift.v1", Data: body,
			Headers: map[string]string{"tenant_id": k.tenant, "model_id": k.model, "severity": severity},
		})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
