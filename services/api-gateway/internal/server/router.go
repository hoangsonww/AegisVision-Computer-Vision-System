// Package server wires the HTTP router and the middleware chain for the
// api-gateway. We use net/http and a small stdlib mux on purpose: the public
// surface is small and stable enough that pulling in a router framework
// would be more churn than benefit.
package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/aegisvision/pkg/platform/idempotency"
	"github.com/aegisvision/pkg/platform/metrics"
	"github.com/aegisvision/pkg/platform/middleware"

	"github.com/aegisvision/services/api-gateway/internal/handlers"

	"log/slog"
)

// Deps is the set of constructed dependencies the router needs.
type Deps struct {
	Logger           *slog.Logger
	Metrics          *metrics.RED
	AuthZ            middleware.AuthZDecider
	// Authn, when non-nil, is run before tenant extraction. In production
	// this is the JWT/JWKS middleware; in dev it is left nil and the
	// X-Tenant-Id header is honored directly.
	Authn            func(http.Handler) http.Handler
	IdempotencyStore idempotency.Store
	IdempotencyTTL   time.Duration

	Pipelines   *handlers.PipelineHandlers
	Streams     *handlers.StreamHandlers   // optional
	EventStream *handlers.EventStreamProxy // optional

	// ConsoleDir is the directory served at /console for the walking
	// skeleton; empty disables the console route entirely.
	ConsoleDir string

	// AllowedOrigins enables CORS for the listed origins. "*" allows any.
	// Leave empty (nil) to disable CORS — the gateway will then reject
	// browser preflight OPTIONS requests.
	AllowedOrigins []string
}

// NewRouter composes the public REST surface. The middleware order is the
// canonical chain documented in pkg/platform/middleware.
func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()

	// Pipelines
	mux.HandleFunc("/v1/pipelines", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			d.Pipelines.List(w, r)
		case http.MethodPost:
			d.Pipelines.Create(w, r)
		default:
			methodNotAllowed(w)
		}
	})
	mux.HandleFunc("/v1/pipelines/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			d.Pipelines.Get(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":deploy"):
			d.Pipelines.Deploy(w, r)
		default:
			methodNotAllowed(w)
		}
	})

	// Streams (optional — wired iff Deps.Streams is non-nil)
	if d.Streams != nil {
		mux.HandleFunc("/v1/streams", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				d.Streams.List(w, r)
			case http.MethodPost:
				d.Streams.Create(w, r)
			default:
				methodNotAllowed(w)
			}
		})
		mux.HandleFunc("/v1/streams/", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				d.Streams.Get(w, r)
			case http.MethodDelete:
				d.Streams.Delete(w, r)
			default:
				methodNotAllowed(w)
			}
		})
	}

	// SSE — wired iff Deps.EventStream is non-nil.
	if d.EventStream != nil {
		mux.Handle("/v1/events:stream", d.EventStream.Handler())
	}

	// Health — public probe for clients (console, monitors). Lives on the
	// API surface (not the dedicated AEGIS_HEALTH_ADDR) so a browser fetch
	// can confirm the gateway is reachable without a separate CORS surface.
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Build the protected middleware chain. SSE is handled separately
	// because Idempotency middleware would buffer the entire response.
	tenantMW := middleware.TenantFromHeader()
	if d.Authn != nil {
		// In production, JWT carries the tenant claim; TenantFromHeader is
		// not the tenant extractor, just a defensive fallback.
		tenantMW = d.Authn
	}
	protected := chainOf(
		middleware.Recovery(d.Logger),
		middleware.RequestID(),
		middleware.Logging(d.Logger),
		d.Metrics.HTTPMiddleware(routeLabel),
		tenantMW,
		middleware.AuthZ(d.AuthZ),
		middleware.Idempotency(d.IdempotencyStore, d.IdempotencyTTL),
	)
	sseChain := chainOf(
		middleware.Recovery(d.Logger),
		middleware.RequestID(),
		middleware.Logging(d.Logger),
		d.Metrics.HTTPMiddleware(routeLabel),
		tenantMW,
		middleware.AuthZ(d.AuthZ),
	)

	root := http.NewServeMux()
	root.Handle("/v1/events:stream", sseChain(mux))
	root.Handle("/v1/", protected(mux))

	// CORS sits in front of everything so browser preflight OPTIONS
	// requests are answered before AuthZ has a chance to 401 them.
	var rootHandler http.Handler = root
	if len(d.AllowedOrigins) > 0 {
		rootHandler = middleware.CORS(d.AllowedOrigins)(root)
	}

	// Console: unauthenticated static files. The console is HTML + JS that
	// itself authenticates via X-Tenant-Id when it talks to /v1/.
	if d.ConsoleDir != "" {
		root.Handle("/console/", http.StripPrefix("/console/", http.FileServer(http.Dir(d.ConsoleDir))))
		root.HandleFunc("/console", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/console/", http.StatusFound)
		})
	}

	return rootHandler
}

func routeLabel(r *http.Request) string {
	p := r.URL.Path
	switch {
	case p == "/v1/pipelines":
		return "/v1/pipelines"
	case strings.HasPrefix(p, "/v1/pipelines/") && strings.HasSuffix(p, ":deploy"):
		return "/v1/pipelines/{id}:deploy"
	case strings.HasPrefix(p, "/v1/pipelines/"):
		return "/v1/pipelines/{id}"
	case p == "/v1/streams":
		return "/v1/streams"
	case strings.HasPrefix(p, "/v1/streams/"):
		return "/v1/streams/{id}"
	case p == "/v1/events:stream":
		return "/v1/events:stream"
	}
	return "other"
}

func chainOf(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, _ = w.Write([]byte(`{"title":"Method Not Allowed","status":405,"code":"method_not_allowed"}`))
}
