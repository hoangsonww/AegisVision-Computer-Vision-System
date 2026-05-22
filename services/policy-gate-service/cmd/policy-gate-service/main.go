// Command policy-gate-service is the human-approval gate for consequential
// agent actions (ADR-0014, ADR-0017).
//
// An agent calls POST /v1/gates with an action summary + tool arguments and
// the request lands in 'pending'. Operators see pending gates via the
// console (GET /v1/gates?tenant=...&decision=pending) and approve or reject
// via POST /v1/gates/{id}/decision.
//
// On any state change we:
//
//   - Write a row update (the request fields are immutable; only decision_*
//     can change — see store.Decide).
//   - Publish an audit record to audit.v1 (append-only).
//   - Publish a `gate.resolved.<gate_id>` event so agent-service can
//     unblock the awaiting session.
//
// This is the Temporal-style signal gate from the architecture doc, simplified
// to an HTTP+NATS surface for Phase 4. When Temporal lands fully (Phase 5),
// this service becomes a Temporal workflow with the same external API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
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

	"github.com/aegisvision/services/policy-gate-service/internal/store"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "policy-gate-service"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8410")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8411")
	dbDriver := config.String("AEGIS_DB_DRIVER", "sqlite")
	dbDSN := config.String("AEGIS_DB_DSN", "file:/tmp/gates.sqlite?_pragma=journal_mode(WAL)")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	natsURL := config.String("AEGIS_NATS_URL", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

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
	opened := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_gates_opened_total", Help: "Gates opened."})
	decided := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_gates_decided_total", Help: "Gates decided."}, []string{"decision"})
	pendingAge := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "aegis_gate_decision_age_seconds", Help: "Age of gate at decision time.",
		Buckets: []float64{1, 10, 60, 300, 1800, 3600, 7200, 21600, 86400},
	})
	reg.MustRegister(opened, decided, pendingAge)

	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(
				middleware.TenantFromHeader()(h)))))
	}

	mux.Handle("POST /v1/gates", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req intelligence.GateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		if req.ID == "" || req.SessionID == "" || req.ToolName == "" {
			problem.BadRequest("id, session_id, tool_name required").Write(w)
			return
		}
		// Tenant from identity overrides payload.
		req.TenantID = logging.TenantID(r.Context())
		if req.BlastRadius == "" {
			req.BlastRadius = "medium"
		}
		gate := intelligence.Gate{Request: req, Decision: intelligence.GateDecisionPending}
		if _, err := st.Open(r.Context(), gate); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		opened.Inc()
		audit(pub, req.TenantID, "gate.opened", gate)
		writeJSON(w, http.StatusCreated, gate)
	}), "/v1/gates"))

	mux.Handle("GET /v1/gates", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		out, err := st.ListPending(r.Context(), tenant, 50)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}), "/v1/gates"))

	mux.Handle("GET /v1/gates/{id}", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		g, err := st.Get(r.Context(), id)
		if err != nil {
			problem.NotFound("gate not found").Write(w)
			return
		}
		// A gate is only readable by its tenant.
		if tenant := logging.TenantID(r.Context()); tenant != "" && tenant != g.Request.TenantID {
			problem.Forbidden("cross-tenant read denied").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, g)
	}), "/v1/gates/{id}"))

	mux.Handle("POST /v1/gates/{id}/decision", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		decision := intelligence.GateDecision(strings.ToLower(body.Decision))
		decidedBy := r.Header.Get("X-Aegis-Subject")
		if decidedBy == "" {
			problem.Forbidden("no identity for decision").Write(w)
			return
		}
		g, changed, err := st.Decide(r.Context(), id, decision, decidedBy, body.Reason)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		if changed {
			decided.WithLabelValues(string(decision)).Inc()
			if !g.Request.RequestedAt.IsZero() {
				pendingAge.Observe(time.Since(g.Request.RequestedAt).Seconds())
			}
			audit(pub, g.Request.TenantID, "gate.decided", g)
			// Publish a resolution signal so agent-service can resume.
			b, _ := json.Marshal(g)
			_ = pub.Publish(r.Context(), bus.Message{
				Subject: fmt.Sprintf("gate.resolved.%s", g.Request.ID),
				Data:    b,
				Headers: map[string]string{"tenant_id": g.Request.TenantID, "session_id": g.Request.SessionID},
			})
		}
		writeJSON(w, http.StatusOK, g)
	}), "/v1/gates/{id}/decision"))

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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func audit(pub bus.Publisher, tenant, kind string, body any) {
	b, _ := json.Marshal(map[string]any{"kind": kind, "body": body, "ts": time.Now().UTC()})
	_ = pub.Publish(context.Background(), bus.Message{
		Subject: "audit.v1",
		Data:    b,
		Headers: map[string]string{"tenant_id": tenant, "category": "policy-gate"},
	})
}

var _ = errors.New
