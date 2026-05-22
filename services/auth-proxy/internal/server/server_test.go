package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/services/auth-proxy/internal/server"
)

func TestCheckHandler_DeniesWithoutTenant(t *testing.T) {
	h := server.CheckHandler("k", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/check", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCheckHandler_SignsIdentity_OnAllow(t *testing.T) {
	h := server.CheckHandler("test-key", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/check", nil).
		WithContext(logging.WithTenantID(context.Background(), "tenant-1"))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	hdr := rec.Header().Get("X-Aegis-Identity")
	if hdr == "" {
		t.Fatal("no identity header")
	}
	tenant, ok := server.VerifyIdentityHeader("test-key", hdr)
	if !ok || tenant != "tenant-1" {
		t.Fatalf("verify: ok=%v tenant=%q", ok, tenant)
	}
}

func TestVerifyIdentityHeader_RejectsWrongKey(t *testing.T) {
	h := server.CheckHandler("right-key", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/check", nil).
		WithContext(logging.WithTenantID(context.Background(), "tenant-1"))
	h.ServeHTTP(rec, req)
	hdr := rec.Header().Get("X-Aegis-Identity")
	if _, ok := server.VerifyIdentityHeader("wrong-key", hdr); ok {
		t.Fatal("expected verification failure with wrong key")
	}
}

func TestVerifyIdentityHeader_RejectsMalformed(t *testing.T) {
	cases := []string{"", "no-dot", "trailing.", ".leading", "only.one"}
	for _, c := range cases {
		if _, ok := server.VerifyIdentityHeader("k", c); ok {
			if c == "only.one" {
				// "only.one" has a dot but neither side is a valid signed payload.
				if _, ok := server.VerifyIdentityHeader("k", c); ok {
					t.Fatalf("expected rejection for %q", c)
				}
			} else {
				t.Fatalf("expected rejection for %q", c)
			}
		}
	}
}
