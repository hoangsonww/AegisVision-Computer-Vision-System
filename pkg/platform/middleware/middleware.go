// Package middleware contains the HTTP middlewares every public-facing service
// composes. Order matters; the canonical chain is:
//
//	Recovery -> RequestID -> Logging -> Metrics -> Auth -> Idempotency -> handler
//
// The chain is intentionally short and concrete. Services that need
// additional middleware (rate limiting per tenant tier, mTLS-peer
// allowlisting, etc.) should write them service-local and plug them in.
package middleware

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/aegisvision/pkg/platform/idempotency"
	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"
)

// Recovery converts a panicking handler into a 500 problem+json response so a
// programmer error in one endpoint never takes down the process.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if logger != nil {
						logger.ErrorContext(r.Context(), "handler panic",
							slog.Any("recover", rec),
							slog.String("stack", string(debug.Stack())),
						)
					}
					problem.Internal("internal server error").Write(w)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequestID accepts an inbound X-Request-Id or mints a fresh one, propagates
// it into the request context, and echoes it back on the response.
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-Id")
			if id == "" {
				id = newID()
			}
			ctx := logging.WithRequestID(r.Context(), id)
			w.Header().Set("X-Request-Id", id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Logging records one structured log line per request. It does NOT log the
// request body — that's the easiest way to leak PII into Loki. If you need
// it during incident triage, sample it explicitly into a separate sink.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			next.ServeHTTP(rec, r)
			logger.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.wroteHeader {
		return
	}
	s.status = code
	s.wroteHeader = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}

// Flush passes through to the underlying ResponseWriter if it supports it.
// SSE handlers depend on this — without it the response sits in the buffer
// until the connection closes and clients see nothing live.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Idempotency wires the idempotency-key contract for mutating endpoints.
//
//   - On GET/HEAD/OPTIONS, the middleware is a no-op.
//   - On POST/PUT/PATCH/DELETE: if Idempotency-Key is set and a cached entry
//     exists, the cached response is replayed verbatim. If the request body
//     fingerprint differs, a 409 problem+json is returned.
//   - If the upstream handler succeeds (2xx), the response is captured and
//     stored for replay within the configured TTL (default 24h per the docs).
func Idempotency(store idempotency.Store, ttl time.Duration) func(http.Handler) http.Handler {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutating(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			tenant := logging.TenantID(r.Context())

			// Read and replace the body so the downstream handler still sees it.
			body, err := io.ReadAll(r.Body)
			if err != nil {
				problem.BadRequest("could not read request body").Write(w)
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
			fp := fingerprint(body)

			if cached, ok, err := store.Get(r.Context(), tenant, key); err == nil && ok {
				if cached.BodyFingerprint != fp {
					problem.Conflict("idempotency-key reused with a different body").Write(w)
					return
				}
				if cached.ContentType != "" {
					w.Header().Set("Content-Type", cached.ContentType)
				}
				w.Header().Set("Idempotent-Replay", "true")
				w.WriteHeader(cached.Status)
				_, _ = w.Write(cached.Body)
				return
			}

			cap := &captureWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(cap, r)

			if cap.status >= 200 && cap.status < 300 {
				_ = store.Put(r.Context(), tenant, key, idempotency.Entry{
					Status:          cap.status,
					ContentType:     w.Header().Get("Content-Type"),
					Body:            cap.buf.Bytes(),
					BodyFingerprint: fp,
					StoredAt:        time.Now(),
					ExpiresAt:       time.Now().Add(ttl),
				})
			}
		})
	}
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func fingerprint(b []byte) string {
	if len(b) == 0 {
		return "empty"
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type captureWriter struct {
	http.ResponseWriter
	buf         bytes.Buffer
	status      int
	wroteHeader bool
}

func (c *captureWriter) WriteHeader(code int) {
	if c.wroteHeader {
		return
	}
	c.status = code
	c.wroteHeader = true
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.wroteHeader = true
	}
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}

// TenantFromHeader extracts the tenant id from the X-Tenant-Id header and
// attaches it to the context. In production the gateway derives this from
// the OIDC token after OPA decides the principal has access; in dev and
// during early-phase tests the header is honored directly.
func TenantFromHeader() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := strings.TrimSpace(r.Header.Get("X-Tenant-Id"))
			if t == "" {
				problem.Unauthorized("X-Tenant-Id is required").Write(w)
				return
			}
			ctx := logging.WithTenantID(r.Context(), t)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthZDecider is the contract OPA implementations satisfy. In production an
// adapter calls out to an OPA sidecar over HTTP; in tests, a closure suffices.
type AuthZDecider interface {
	Allow(ctx context.Context, input AuthZInput) (bool, error)
}

type AuthZInput struct {
	Tenant string
	Method string
	Path   string
	Action string
	Roles  []string
}

// AuthZDeciderFunc adapts a plain function into an AuthZDecider.
type AuthZDeciderFunc func(context.Context, AuthZInput) (bool, error)

func (f AuthZDeciderFunc) Allow(ctx context.Context, in AuthZInput) (bool, error) {
	return f(ctx, in)
}

// ErrAuthZUnavailable should be returned by deciders when the policy engine
// can't be reached; AuthZ treats that as fail-closed and emits 503 so the
// caller can retry. Per the doc: deny-by-default.
var ErrAuthZUnavailable = errors.New("authz engine unavailable")

// AuthZ rejects any request the decider disallows. Deny-by-default applies
// to ambiguous or error responses.
func AuthZ(decider AuthZDecider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			in := AuthZInput{
				Tenant: logging.TenantID(r.Context()),
				Method: r.Method,
				Path:   r.URL.Path,
				Action: actionFromMethod(r.Method),
			}
			ok, err := decider.Allow(r.Context(), in)
			if errors.Is(err, ErrAuthZUnavailable) {
				problem.ServiceUnavailable("authorization engine unavailable").Write(w)
				return
			}
			if err != nil {
				problem.Forbidden("authorization decision error").Write(w)
				return
			}
			if !ok {
				problem.Forbidden("not allowed").Write(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func actionFromMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead:
		return "read"
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	}
	return "unknown"
}
