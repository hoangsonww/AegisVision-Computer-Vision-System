// Audit middleware emits one audit.v1 record per mutating request.
//
// Per ADR-0014, every consequential action — by humans or agents — must be
// auditable. The shape is the AuditRecord protobuf in proto/common/v1; we
// emit it as JSON here to keep middleware decoupled from generated proto
// types (the audit-service deserializes either form).
//
// The middleware fires AFTER the handler completes so the recorded
// `outcome` reflects the result. Failed authorizations are emitted too —
// for compliance you must be able to prove that an attempt was made and
// rejected.
package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aegisvision/pkg/platform/logging"
)

// AuditPublisher is the minimum surface the audit middleware needs.
type AuditPublisher interface {
	Publish(ctx context.Context, subject string, body []byte) error
}

// AuditConfig configures the middleware.
type AuditConfig struct {
	Publisher AuditPublisher
	Subject   string // defaults to "audit.v1"
	Service   string // service name recorded as the action source
	Logger    *slog.Logger
}

// AuditRecord is the JSON wire form. Keep field names stable; downstream
// consumers (audit-service, SIEM exporters) depend on them.
type AuditRecord struct {
	ID        string            `json:"id"`
	At        time.Time         `json:"at"`
	Service   string            `json:"service"`
	TenantID  string            `json:"tenant_id,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Status    int               `json:"status"`
	Outcome   string            `json:"outcome"` // SUCCESS | DENIED | ERROR
	UserAgent string            `json:"user_agent,omitempty"`
	RemoteIP  string            `json:"remote_ip,omitempty"`
	Verb      string            `json:"verb,omitempty"`
	Resource  string            `json:"resource,omitempty"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// Audit returns middleware that emits one AuditRecord per mutating request.
// Read-only requests are not audited at the gateway; tenant-scoped read
// access is logged separately.
func Audit(cfg AuditConfig) func(http.Handler) http.Handler {
	if cfg.Subject == "" {
		cfg.Subject = "audit.v1"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutating(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			next.ServeHTTP(rec, r)
			outcome := classify(rec.status)
			body, _ := json.Marshal(AuditRecord{
				ID:        newAuditID(),
				At:        time.Now().UTC(),
				Service:   cfg.Service,
				TenantID:  logging.TenantID(r.Context()),
				RequestID: logging.RequestID(r.Context()),
				Method:    r.Method,
				Path:      r.URL.Path,
				Status:    rec.status,
				Outcome:   outcome,
				UserAgent: r.UserAgent(),
				RemoteIP:  remoteIP(r),
				Verb:      verbFromMethod(r.Method),
				Resource:  resourceFromPath(r.URL.Path),
			})
			if cfg.Publisher != nil {
				_ = cfg.Publisher.Publish(r.Context(), cfg.Subject, body)
			}
			if cfg.Logger != nil {
				cfg.Logger.InfoContext(r.Context(), "audit",
					slog.String("verb", verbFromMethod(r.Method)),
					slog.String("path", r.URL.Path),
					slog.Int("status", rec.status),
					slog.String("outcome", outcome),
				)
			}
		})
	}
}

func classify(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "SUCCESS"
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "DENIED"
	default:
		return "ERROR"
	}
}

func verbFromMethod(m string) string {
	switch m {
	case http.MethodPost:
		return "Create"
	case http.MethodPut, http.MethodPatch:
		return "Update"
	case http.MethodDelete:
		return "Delete"
	}
	return m
}

// resourceFromPath collapses /v1/pipelines/{id}:deploy → /v1/pipelines/* —
// useful for grouping audit dashboards.
func resourceFromPath(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) <= 2 {
		return p
	}
	parts[2] = "*"
	return "/" + strings.Join(parts, "/")
}

func remoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	return r.RemoteAddr
}

func newAuditID() string {
	// Use the same hex pattern as request IDs — short, time-prefix not
	// needed because the AuditRecord carries `at` already.
	return newID()
}
