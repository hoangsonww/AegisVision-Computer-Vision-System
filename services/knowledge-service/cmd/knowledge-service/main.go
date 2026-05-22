// Command knowledge-service is the platform's RAG corpus (ADR-0020).
//
// Documents are ingested from docs/ on startup (and on-demand via
// /v1/knowledge/reingest in dev). Search is cosine over embeddings, with
// pgvector when available and brute-force fallback otherwise.
//
// Per-tenant collections: tenant_id="" docs are platform-global (ADRs,
// runbooks, README, compliance). Tenant-specific docs are searchable only
// by that tenant. Cross-tenant reads are forbidden.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aegisvision/pkg/embeddings"
	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/pkg/llm"
	"github.com/aegisvision/pkg/platform/config"
	"github.com/aegisvision/pkg/platform/health"
	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/middleware"
	"github.com/aegisvision/pkg/platform/otelinit"
	"github.com/aegisvision/pkg/platform/problem"
	"github.com/aegisvision/pkg/platform/shutdown"
	"github.com/aegisvision/pkg/store/sqldb"

	"github.com/aegisvision/services/knowledge-service/internal/ingest"

	"github.com/prometheus/client_golang/prometheus"
)

const serviceName = "knowledge-service"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	env := config.String("AEGIS_ENV", "dev")
	version := config.String("AEGIS_VERSION", "0.0.0-dev")
	httpAddr := config.String("AEGIS_HTTP_ADDR", ":8430")
	healthAddr := config.String("AEGIS_HEALTH_ADDR", ":8431")
	dbDriver := config.String("AEGIS_DB_DRIVER", "sqlite")
	dbDSN := config.String("AEGIS_DB_DSN", "file:/tmp/knowledge.sqlite?_pragma=journal_mode(WAL)")
	llmURL := config.String("AEGIS_LLM_GATEWAY_URL", "")
	embModel := config.String("AEGIS_EMBED_MODEL", "default")
	docsRoot := config.String("AEGIS_DOCS_ROOT", "/docs")
	otlpEndpoint := config.String("AEGIS_OTLP_ENDPOINT", "")
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
	backendName := "sqlite"
	if dbDriver == sqldb.DriverPostgres {
		backendName = "postgres"
	}
	st := embeddings.New(db, backendName)
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	var llmClient llm.Client
	if llmURL == "" {
		llmClient = llm.NewFakeClient()
		logger.Warn("AEGIS_LLM_GATEWAY_URL empty — embeddings use fake client (dev only)")
	} else {
		llmClient = llm.NewHTTPClient(llmURL)
	}
	ing := ingest.New(st, llmClient, embModel)

	// Ingest at startup (best-effort).
	if _, err := os.Stat(docsRoot); err == nil {
		n, _ := ing.IngestDir(ctx, docsRoot)
		logger.Info("ingested docs", slog.String("root", docsRoot), slog.Int("chunks", n))
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	red := metrics.New(serviceName, reg)
	docsTotal := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_knowledge_docs_ingested_total", Help: "Docs ingested."})
	searchTotal := prometheus.NewCounter(prometheus.CounterOpts{Name: "aegis_knowledge_search_total", Help: "Search queries."})
	reg.MustRegister(docsTotal, searchTotal)

	mux := http.NewServeMux()
	wrap := func(h http.Handler, route string) http.Handler {
		return middleware.Recovery(logger)(middleware.RequestID()(middleware.Logging(logger)(
			red.HTTPMiddleware(func(*http.Request) string { return route })(
				middleware.TenantFromHeader()(h)))))
	}

	mux.Handle("GET /v1/knowledge/search", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			problem.BadRequest("q required").Write(w)
			return
		}
		tenant := logging.TenantID(r.Context())
		limit := atoi(r.URL.Query().Get("limit"), 5)
		var kinds []intelligence.DocKind
		for _, k := range r.URL.Query()["kind"] {
			kinds = append(kinds, intelligence.DocKind(k))
		}
		var requireTags []string
		if tag := r.URL.Query().Get("tag"); tag != "" {
			requireTags = strings.Split(tag, ",")
		}
		vec, err := embeddings.One(r.Context(), llmClient, embModel, tenant, q)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		hits, err := st.Search(r.Context(), tenant, vec, limit, kinds, requireTags)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		searchTotal.Inc()
		writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
	}), "/v1/knowledge/search"))

	mux.Handle("POST /v1/knowledge/docs", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var doc intelligence.Doc
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		// Tenant from identity; null tenant_id is reserved for platform-global,
		// and creating those requires the platform admin role (we approximate
		// with header — wire to OPA when available).
		tenant := logging.TenantID(r.Context())
		if doc.TenantID == "" {
			doc.TenantID = tenant
		}
		if doc.TenantID != tenant && r.Header.Get("X-Aegis-Role") != "platform-admin" {
			problem.Forbidden("cross-tenant write denied").Write(w)
			return
		}
		if doc.ID == "" {
			doc.ID = fmt.Sprintf("doc-%d", time.Now().UnixNano())
		}
		if doc.IngestedAt.IsZero() {
			doc.IngestedAt = time.Now().UTC()
		}
		vec, err := embeddings.One(r.Context(), llmClient, embModel, doc.TenantID, doc.Body)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		if err := st.Upsert(r.Context(), doc, vec); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		docsTotal.Inc()
		writeJSON(w, http.StatusCreated, doc)
	}), "/v1/knowledge/docs"))

	mux.Handle("POST /v1/knowledge/reingest", wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if env != "dev" && r.Header.Get("X-Aegis-Role") != "platform-admin" {
			problem.Forbidden("reingest is admin-only outside dev").Write(w)
			return
		}
		n, err := ing.IngestDir(r.Context(), docsRoot)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"chunks_ingested": n})
	}), "/v1/knowledge/reingest"))

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

func atoi(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return def
	}
	return n
}
