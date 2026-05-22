// Command active-learning-service runs the uncertainty-driven sampling
// loop. Subscribes to `detections.v1`, scores each detection's predicted
// distribution under the tenant's learning policy, and — if accepted —
// claim-checks a frame snapshot, queues an annotation task, and records a
// Sample row.
//
// The labeled feedback re-enters via /v1/learning/samples/{id}/labels,
// which marks the sample 'labeled' and emits a `learning.labeled.v1`
// event consumed by training-orchestrator.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"sync"
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

	"github.com/aegisvision/services/active-learning-service/internal/sampler"
	"github.com/aegisvision/services/active-learning-service/internal/store"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "active-learning-service"

// DetectionEvent is the platform-internal shape of detections.v1 used by
// the active-learning loop. Only the fields we actually need are listed —
// upstream is free to add more.
type DetectionEvent struct {
	TenantID   string             `json:"tenant_id"`
	ModelID    string             `json:"model_id"`
	StreamID   string             `json:"stream_id"`
	FrameURN   string             `json:"frame_urn"` // claim-check ref; we only ever hold the URN
	Topology   string             `json:"topology"`  // model topology hash
	Predicted  string             `json:"predicted_class"`
	Confidence float64            `json:"confidence"`
	ClassProbs map[string]float64 `json:"class_probs"`
	OccurredAt time.Time          `json:"occurred_at"`
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
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8440")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8441")
	dbDriver := config.String("AEGIS_DB_DRIVER", "sqlite")
	dbDSN := config.String("AEGIS_DB_DSN", "file:/tmp/learning.sqlite?_pragma=journal_mode(WAL)")
	natsURL := config.String("AEGIS_NATS_URL", "")
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

	// In-memory budgeters keyed by policy-id+day (the SQL CountToday gates
	// across restarts; this layer is for the hot path).
	budgetersMu := &sync.Mutex{}
	budgeters := map[string]*sampler.Budgeter{}
	dayOf := func(t time.Time) string { return t.UTC().Format("2006-01-02") }
	getBudget := func(policyID string, now time.Time) *sampler.Budgeter {
		k := policyID + "/" + dayOf(now)
		budgetersMu.Lock()
		defer budgetersMu.Unlock()
		b, ok := budgeters[k]
		if !ok {
			b = sampler.NewBudgeter()
			budgeters[k] = b
		}
		return b
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	considered := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_al_detections_considered_total", Help: "Detections considered."})
	sampled := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_al_samples_queued_total", Help: "Samples queued for annotation."})
	rejected := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "aegis_al_samples_rejected_total", Help: "Samples rejected by sampler."}, []string{"reason"})
	labeled := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_al_samples_labeled_total", Help: "Samples labeled by humans."})
	reg.MustRegister(considered, sampled, rejected, labeled)

	// Subscribe to the detection firehose.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rngMu := &sync.Mutex{}
	handler := func(ctx context.Context, msg bus.Message) error {
		considered.Inc()
		var ev DetectionEvent
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			return nil // bad payloads are not retryable
		}
		policies, _ := st.ListPoliciesByModel(ctx, ev.TenantID, ev.ModelID)
		for _, p := range policies {
			// Probabilistic gate.
			rngMu.Lock()
			roll := rng.Float64()
			rngMu.Unlock()
			if roll > p.SampleRate {
				continue
			}
			probs := make([]float64, 0, len(ev.ClassProbs))
			for _, v := range ev.ClassProbs {
				probs = append(probs, v)
			}
			score := sampler.Score(p.Strategy, probs)
			if !sampler.InBand(score, p) {
				rejected.WithLabelValues("out_of_band").Inc()
				continue
			}
			b := getBudget(p.ID, time.Now())
			d, ok := b.Try(ev.Predicted, p)
			if !ok {
				rejected.WithLabelValues(d.Reason).Inc()
				continue
			}
			id := fmt.Sprintf("smp-%d-%s", time.Now().UnixNano(), randSuffix(rng, rngMu))
			sm := intelligence.Sample{
				ID: id, TenantID: ev.TenantID, ModelID: ev.ModelID, PolicyID: p.ID,
				SnapshotURN: ev.FrameURN, PredictedClass: ev.Predicted,
				PredictedConfidence: ev.Confidence, Uncertainty: score,
				Status:    intelligence.SampleStatusQueued,
				SampledAt: time.Now().UTC(),
			}
			if err := st.InsertSample(ctx, sm); err != nil {
				logger.WarnContext(ctx, "insert sample", slog.String("err", err.Error()))
				continue
			}
			sampled.Inc()
			// Publish a learning.sampled.v1 event for annotation-service to
			// pick up + enqueue.
			out, _ := json.Marshal(sm)
			_ = pub.Publish(ctx, bus.Message{
				Subject: "learning.sampled.v1",
				Data:    out,
				Headers: map[string]string{"tenant_id": sm.TenantID, "policy_id": sm.PolicyID},
			})
		}
		return nil
	}
	if _, err := sub.Subscribe(ctx, "detections.v1", "active-learning", handler); err != nil {
		return err
	}

	// HTTP surface: CRUD on policies + samples.
	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(
				middleware.TenantFromHeader()(h)))))
	}

	mux.Handle("POST /v1/learning/policies", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p intelligence.LearningPolicy
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		p.TenantID = logging.TenantID(r.Context())
		if p.ID == "" {
			p.ID = fmt.Sprintf("lp-%d", time.Now().UnixNano())
		}
		if err := st.UpsertPolicy(r.Context(), p); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, p)
	}), "/v1/learning/policies"))

	mux.Handle("GET /v1/learning/samples", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		statusFilter := r.URL.Query().Get("status")
		out, err := st.ListSamples(r.Context(), tenant, statusFilter, 100)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}), "/v1/learning/samples"))

	mux.Handle("POST /v1/learning/samples/{id}/labels", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			FinalClass string `json:"final_class"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		id := r.PathValue("id")
		if body.FinalClass == "" {
			problem.BadRequest("final_class required").Write(w)
			return
		}
		if err := st.MarkLabeled(r.Context(), id, body.FinalClass); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		labeled.Inc()
		out, _ := json.Marshal(map[string]any{"sample_id": id, "final_class": body.FinalClass})
		_ = pub.Publish(r.Context(), bus.Message{
			Subject: "learning.labeled.v1", Data: out,
			Headers: map[string]string{"tenant_id": logging.TenantID(r.Context())},
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}), "/v1/learning/samples/{id}/labels"))

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

func randSuffix(rng *rand.Rand, mu *sync.Mutex) string {
	mu.Lock()
	defer mu.Unlock()
	const alpha = "0123456789abcdef"
	b := make([]byte, 6)
	for i := range b {
		b[i] = alpha[rng.Intn(len(alpha))]
	}
	return string(b)
}
