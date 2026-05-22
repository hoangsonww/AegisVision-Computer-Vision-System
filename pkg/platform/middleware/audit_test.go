package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aegisvision/pkg/platform/middleware"
)

type collector struct {
	mu      sync.Mutex
	records []middleware.AuditRecord
}

func (c *collector) Publish(_ context.Context, _ string, body []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var rec middleware.AuditRecord
	_ = json.Unmarshal(body, &rec)
	c.records = append(c.records, rec)
	return nil
}

func TestAudit_EmitsOnMutating(t *testing.T) {
	c := &collector{}
	h := middleware.TenantFromHeader()(middleware.Audit(middleware.AuditConfig{
		Publisher: c, Service: "test-svc",
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})))

	req := httptest.NewRequest("POST", "/v1/pipelines", strings.NewReader(`{}`))
	req.Header.Set("X-Tenant-Id", "t-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.records) != 1 {
		t.Fatalf("want 1 audit, got %d", len(c.records))
	}
	r := c.records[0]
	if r.Outcome != "SUCCESS" || r.Method != "POST" || r.TenantID != "t-1" || r.Verb != "Create" {
		t.Fatalf("bad record: %+v", r)
	}
}

func TestAudit_SkipsReads(t *testing.T) {
	c := &collector{}
	h := middleware.TenantFromHeader()(middleware.Audit(middleware.AuditConfig{
		Publisher: c, Service: "test",
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	req := httptest.NewRequest("GET", "/v1/pipelines", nil)
	req.Header.Set("X-Tenant-Id", "t-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.records) != 0 {
		t.Fatalf("GET must not audit, got %d", len(c.records))
	}
}

func TestAudit_ClassifiesDeniedOnForbidden(t *testing.T) {
	c := &collector{}
	h := middleware.TenantFromHeader()(middleware.Audit(middleware.AuditConfig{
		Publisher: c, Service: "test",
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})))
	req := httptest.NewRequest("POST", "/v1/pipelines", strings.NewReader(`{}`))
	req.Header.Set("X-Tenant-Id", "t-1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.records[0].Outcome != "DENIED" {
		t.Fatalf("outcome: %q", c.records[0].Outcome)
	}
}
