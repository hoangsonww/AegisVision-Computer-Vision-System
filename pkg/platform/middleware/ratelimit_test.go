package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aegisvision/pkg/platform/middleware"
)

func TestRateLimit_AllowsUnderLimit(t *testing.T) {
	rl := middleware.RateLimit(middleware.RateLimitConfig{
		Metric: "x",
		Provider: middleware.QuotaProviderFunc(func(context.Context, string, string) (int64, error) {
			return 5, nil
		}),
	})
	h := middleware.TenantFromHeader()(rl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/v1/x", nil)
		req.Header.Set("X-Tenant-Id", "t-1")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("under limit: want 200, got %d at iter %d", rr.Code, i)
		}
	}
}

func TestRateLimit_RejectsOverLimit(t *testing.T) {
	rl := middleware.RateLimit(middleware.RateLimitConfig{
		Metric: "x",
		Provider: middleware.QuotaProviderFunc(func(context.Context, string, string) (int64, error) {
			return 2, nil
		}),
	})
	h := middleware.TenantFromHeader()(rl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	codes := []int{}
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest("GET", "/v1/x", nil)
		req.Header.Set("X-Tenant-Id", "t-1")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		codes = append(codes, rr.Code)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != 429 || codes[3] != 429 {
		t.Fatalf("expected 200,200,429,429; got %v", codes)
	}
}

func TestRateLimit_TenantsIsolated(t *testing.T) {
	rl := middleware.RateLimit(middleware.RateLimitConfig{
		Metric: "x",
		Provider: middleware.QuotaProviderFunc(func(context.Context, string, string) (int64, error) {
			return 1, nil
		}),
		Window: time.Minute,
	})
	h := middleware.TenantFromHeader()(rl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	// t-a exhausts its quota; t-b should remain unaffected.
	req1 := httptest.NewRequest("GET", "/v1/x", nil)
	req1.Header.Set("X-Tenant-Id", "t-a")
	httptest.NewRecorder().Result()
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req1)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req1) // second request from t-a, should 429
	if rr2.Code != 429 {
		t.Fatalf("t-a 2nd: want 429, got %d", rr2.Code)
	}
	req2 := httptest.NewRequest("GET", "/v1/x", nil)
	req2.Header.Set("X-Tenant-Id", "t-b")
	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req2)
	if rr3.Code != 200 {
		t.Fatalf("t-b 1st: want 200, got %d", rr3.Code)
	}
}
