package middleware_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aegisvision/pkg/platform/idempotency"
	"github.com/aegisvision/pkg/platform/middleware"
)

func TestRequestID_AssignsAndEchoes(t *testing.T) {
	h := middleware.RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("mints when absent", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/pipelines", nil))
		if got := rr.Header().Get("X-Request-Id"); got == "" {
			t.Fatal("expected X-Request-Id to be set")
		}
	})

	t.Run("preserves when present", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/pipelines", nil)
		req.Header.Set("X-Request-Id", "abc-123")
		h.ServeHTTP(rr, req)
		if got := rr.Header().Get("X-Request-Id"); got != "abc-123" {
			t.Fatalf("want abc-123, got %q", got)
		}
	})
}

func TestRecovery_Returns500ProblemJSON(t *testing.T) {
	h := middleware.Recovery(nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("want problem+json, got %q", ct)
	}
}

func TestIdempotency_ReplaysSecondCall(t *testing.T) {
	store := idempotency.NewMemoryStore(0)
	called := 0
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"p-001"}`))
	})
	h := middleware.TenantFromHeader()(middleware.Idempotency(store, time.Hour)(upstream))

	body := strings.NewReader(`{"name":"x"}`)
	req1 := httptest.NewRequest("POST", "/v1/pipelines", body)
	req1.Header.Set("X-Tenant-Id", "tenant-a")
	req1.Header.Set("Idempotency-Key", "key-1")
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusCreated {
		t.Fatalf("first call: want 201, got %d", rr1.Code)
	}
	if called != 1 {
		t.Fatalf("first call: upstream should run, got called=%d", called)
	}

	body2 := strings.NewReader(`{"name":"x"}`)
	req2 := httptest.NewRequest("POST", "/v1/pipelines", body2)
	req2.Header.Set("X-Tenant-Id", "tenant-a")
	req2.Header.Set("Idempotency-Key", "key-1")
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusCreated {
		t.Fatalf("replay: want 201, got %d", rr2.Code)
	}
	if called != 1 {
		t.Fatalf("replay: upstream must NOT run, got called=%d", called)
	}
	if rr2.Header().Get("Idempotent-Replay") != "true" {
		t.Fatal("replay: missing Idempotent-Replay marker")
	}
	gotBody, _ := io.ReadAll(rr2.Body)
	if string(gotBody) != `{"id":"p-001"}` {
		t.Fatalf("replay body mismatch: %q", gotBody)
	}
}

func TestIdempotency_ConflictOnBodyChange(t *testing.T) {
	store := idempotency.NewMemoryStore(0)
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	})
	h := middleware.TenantFromHeader()(middleware.Idempotency(store, time.Hour)(upstream))

	for _, payload := range []string{`{"a":1}`, `{"a":2}`} {
		req := httptest.NewRequest("POST", "/v1/pipelines", strings.NewReader(payload))
		req.Header.Set("X-Tenant-Id", "tenant-a")
		req.Header.Set("Idempotency-Key", "same-key")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if payload == `{"a":1}` && rr.Code != http.StatusCreated {
			t.Fatalf("first: want 201, got %d", rr.Code)
		}
		if payload == `{"a":2}` {
			if rr.Code != http.StatusConflict {
				t.Fatalf("body change: want 409, got %d", rr.Code)
			}
			var p map[string]any
			_ = json.Unmarshal(rr.Body.Bytes(), &p)
			if p["code"] != "conflict" {
				t.Fatalf("unexpected problem body: %v", p)
			}
		}
	}
}

func TestAuthZ_DenyByDefault(t *testing.T) {
	deny := middleware.AuthZDeciderFunc(func(context.Context, middleware.AuthZInput) (bool, error) {
		return false, nil
	})
	h := middleware.AuthZ(deny)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/pipelines", nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rr.Code)
	}
}

func TestAuthZ_UnavailableIsFailClosed(t *testing.T) {
	dec := middleware.AuthZDeciderFunc(func(context.Context, middleware.AuthZInput) (bool, error) {
		return false, middleware.ErrAuthZUnavailable
	})
	h := middleware.AuthZ(dec)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/pipelines", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}
