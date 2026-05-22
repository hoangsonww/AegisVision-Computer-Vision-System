// Command autonomy-orchestrator is the Phase-5 control loop (ADR-0022).
//
// It owns Schedule resources (cron-fired + signal-fired) and translates
// each fire into a regular agent-service session. The agent runs with a
// constrained tool catalog (per the schedule's role) and the same
// bounded-autonomy semantics as user-facing agents — tier-3 actions are
// routed through policy-gate-service.
//
// Subscriptions:
//   - autonomy.signal.drift.v1     — signal-fired schedules with prefix "autonomy.signal.drift"
//   - autonomy.signal.slo.v1       — same for SLO
//   - canary.rollback.v1           — reliability agents respond to rollbacks
//
// Each accepted fire records an AutonomyRun row. Long-running sessions
// (those blocked on a gate) leave the run in 'awaiting_gate'; when the
// gate resolves, agent-service resumes and emits its own status, which
// the orchestrator reflects back into the run via a sweeper.
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

	"github.com/aegisvision/services/autonomy-orchestrator/internal/agentclient"
	"github.com/aegisvision/services/autonomy-orchestrator/internal/store"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "autonomy-orchestrator"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8510")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8511")
	dbDriver := config.String("AEGIS_DB_DRIVER", "sqlite")
	dbDSN := config.String("AEGIS_DB_DSN", "file:/tmp/autonomy.sqlite?_pragma=journal_mode(WAL)")
	natsURL := config.String("AEGIS_NATS_URL", "")
	agentURL := config.String("AEGIS_AGENT_SERVICE_URL", "")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
	logLevel := config.String("AEGIS_LOG_LEVEL", "info")

	if env != "dev" && env != "test" {
		if agentURL == "" {
			return errors.New("AEGIS_AGENT_SERVICE_URL is required outside dev")
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

	ac := agentclient.New(agentURL)

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	firesTotal := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_autonomy_fires_total", Help: "Schedule fires."}, []string{"trigger"})
	runsByStatus := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_autonomy_runs_total", Help: "Runs by terminal status."}, []string{"status"})
	gatesOpened := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_autonomy_gates_opened_total", Help: "Gates opened by autonomy agents."})
	reg.MustRegister(firesTotal, runsByStatus, gatesOpened)

	sched := autonomy.NewScheduler()
	router := autonomy.NewSignalRouter()

	// Hot map of schedule_id → schedule, refreshed on every change.
	schedulesMu := &sync.RWMutex{}
	scheduleByID := map[string]intelligence.Schedule{}
	startFire := func(s intelligence.Schedule, trigger intelligence.TriggerKind, reason string) autonomy.Handler {
		return func(ctx context.Context, f autonomy.Fire) error {
			runID := fmt.Sprintf("ar-%d", time.Now().UnixNano())
			r := intelligence.AutonomyRun{
				ID: runID, ScheduleID: s.ID, TenantID: s.TenantID,
				Role: s.Role, Trigger: trigger,
				Status: intelligence.RunStatusRunning, TriggerReason: reason,
				StartedAt: time.Now().UTC(),
			}
			if err := st.InsertRun(ctx, r); err != nil {
				return err
			}
			firesTotal.WithLabelValues(string(trigger)).Inc()
			out, err := ac.StartSession(ctx, s.TenantID, agentclient.StartRequest{
				Role:     string(s.Role),
				Goal:     goalForRole(s.Role, reason),
				MaxSteps: s.MaxSteps,
			})
			if err != nil {
				logger.WarnContext(ctx, "agent start failed", slog.String("err", err.Error()), slog.String("run", runID))
				_ = st.UpdateRunStatus(ctx, runID, intelligence.RunStatusFailed, "", "")
				runsByStatus.WithLabelValues("failed").Inc()
				return err
			}
			status := mapStatus(out.Status)
			if status == intelligence.RunStatusAwaitingGate && out.GateID != "" {
				gatesOpened.Inc()
			}
			_ = st.UpdateRunStatus(ctx, runID, status, out.SessionID, out.GateID)
			runsByStatus.WithLabelValues(string(status)).Inc()
			return nil
		}
	}

	// Wire scheduler + router from the persisted schedules.
	loadAll := func() error {
		all, err := st.ListSchedules(ctx)
		if err != nil {
			return err
		}
		schedulesMu.Lock()
		scheduleByID = map[string]intelligence.Schedule{}
		for _, s := range all {
			scheduleByID[s.ID] = s
		}
		schedulesMu.Unlock()
		for _, s := range all {
			if s.Paused {
				continue
			}
			switch s.Trigger {
			case intelligence.TriggerCron:
				c, perr := autonomy.ParseCron(s.CronExpression)
				if perr != nil {
					logger.Warn("bad cron", slog.String("schedule", s.ID), slog.String("err", perr.Error()))
					continue
				}
				sched.Register(ctx, &autonomy.Entry{
					ID: s.ID, TenantID: s.TenantID, Cron: c,
					MaxJitter: 30 * time.Second, PreventOverlap: true,
				}, startFire(s, intelligence.TriggerCron, "cron tick"))
			case intelligence.TriggerSignal:
				if s.SignalSubject == "" {
					continue
				}
				router.Register(s.SignalSubject, startFire(s, intelligence.TriggerSignal, "signal"))
			}
		}
		return nil
	}
	if err := loadAll(); err != nil {
		return err
	}

	// Subscribe to platform signal subjects.
	for _, subj := range []string{"autonomy.signal.drift.>", "autonomy.signal.slo.>", "canary.rollback.>"} {
		_, _ = sub.Subscribe(ctx, subj, "autonomy-orchestrator", func(ctx context.Context, msg bus.Message) error {
			tenant := msg.Headers["tenant_id"]
			// Find a schedule whose SignalSubject is a prefix of msg.Subject + matches tenant.
			schedulesMu.RLock()
			defer schedulesMu.RUnlock()
			for _, s := range scheduleByID {
				if s.Paused || s.Trigger != intelligence.TriggerSignal {
					continue
				}
				if s.TenantID != "" && s.TenantID != tenant {
					continue
				}
				if !strings.HasPrefix(msg.Subject, s.SignalSubject) {
					continue
				}
				router.Route(ctx, msg.Subject, tenant, s.ID)
			}
			return nil
		})
	}

	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(
				middleware.TenantFromHeader()(h)))))
	}

	mux.Handle("POST /v1/autonomy/schedules", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var s intelligence.Schedule
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		s.TenantID = logging.TenantID(r.Context())
		if s.ID == "" {
			s.ID = fmt.Sprintf("sch-%d", time.Now().UnixNano())
		}
		if s.MaxSteps == 0 {
			s.MaxSteps = 8
		}
		if s.MaxAutonomyTier == 0 {
			s.MaxAutonomyTier = 2
		}
		if err := st.UpsertSchedule(r.Context(), s); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		// Reload (simple — schedule changes are infrequent).
		_ = loadAll()
		writeJSON(w, http.StatusCreated, s)
	}), "/v1/autonomy/schedules"))

	mux.Handle("GET /v1/autonomy/schedules", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Surface only the caller's tenant.
		all, _ := st.ListSchedules(r.Context())
		tenant := logging.TenantID(r.Context())
		out := make([]intelligence.Schedule, 0, len(all))
		for _, s := range all {
			if tenant == "" || s.TenantID == tenant {
				out = append(out, s)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}), "/v1/autonomy/schedules"))

	mux.Handle("GET /v1/autonomy/runs", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, _ := st.ListRuns(r.Context(), logging.TenantID(r.Context()), 100)
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}), "/v1/autonomy/runs"))

	// Manual trigger for an existing schedule (TriggerKind=manual).
	mux.Handle("POST /v1/autonomy/schedules/{id}/trigger", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, err := st.GetSchedule(r.Context(), r.PathValue("id"))
		if err != nil {
			problem.NotFound("schedule not found").Write(w)
			return
		}
		if tenant := logging.TenantID(r.Context()); tenant != "" && tenant != s.TenantID {
			problem.Forbidden("cross-tenant denied").Write(w)
			return
		}
		_ = startFire(*s, intelligence.TriggerManual, "manual")(r.Context(), autonomy.Fire{ScheduleID: s.ID, TenantID: s.TenantID, At: time.Now()})
		w.WriteHeader(http.StatusAccepted)
	}), "/v1/autonomy/schedules/{id}:trigger"))

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
	runner.Register("scheduler", func(ctx context.Context) error { return sched.Close(ctx) })
	runner.Register("otel", func(ctx context.Context) error { return otelShutdown(ctx) })

	go func() { _ = httpSrv.ListenAndServe() }()
	go func() { _ = hsrv.ListenAndServe() }()
	logger.Info("started", slog.String("addr", httpAddr))
	return runner.Wait(ctx)
}

func mapStatus(s string) intelligence.RunStatus {
	switch s {
	case "completed":
		return intelligence.RunStatusCompleted
	case "awaiting_gate":
		return intelligence.RunStatusAwaitingGate
	case "failed":
		return intelligence.RunStatusFailed
	default:
		return intelligence.RunStatusRunning
	}
}

func goalForRole(role intelligence.AutonomyAgentRole, reason string) string {
	switch role {
	case intelligence.AutonomyRoleOptimizer:
		return "Periodic optimizer run (" + reason + "). Audit cost lines for the past 24h; propose reductions; cite tenant_id + pipeline_id."
	case intelligence.AutonomyRoleMonitor:
		return "Monitor run (" + reason + "). Check SLO + drift signals; summarize anomalies; cite signal ids."
	case intelligence.AutonomyRoleTrainer:
		return "Trainer run (" + reason + "). Check labeled samples; recommend a training job if thresholds are met."
	case intelligence.AutonomyRoleReliability:
		return "Reliability run (" + reason + "). Investigate the rollback; produce an incident summary; cite plan + transition ids."
	}
	return "Scheduled run (" + reason + ")."
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
