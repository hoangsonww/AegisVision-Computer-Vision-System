package middleware_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aegisvision/pkg/platform/middleware"
)

// signRS256 produces a valid RS256 JWT for testing. We avoid a third-party
// JWT library so the middleware is unit-testable without expanding the
// dependency surface.
func signRS256(t *testing.T, priv *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	hdr, _ := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT", "kid": kid})
	payload, _ := json.Marshal(claims)
	signed := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(payload)
	h := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, 5 /* crypto.SHA256 */, h[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func rsaJWKS(t *testing.T, priv *rsa.PrivateKey, kid string) []byte {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(priv.N.Bytes())
	eBytes := []byte{0x01, 0x00, 0x01} // 65537
	e := base64.RawURLEncoding.EncodeToString(eBytes)
	body, _ := json.Marshal(map[string]any{
		"keys": []map[string]any{
			{"kid": kid, "kty": "RSA", "alg": "RS256", "use": "sig", "n": n, "e": e},
		},
	})
	return body
}

func TestAuthn_AcceptsValidToken(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "test-kid"
	jwks := rsaJWKS(t, priv, kid)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	}))
	defer ts.Close()

	authn, err := middleware.Authn(middleware.AuthnConfig{
		JWKSURL:        ts.URL,
		Issuer:         "https://issuer.example",
		Audience:       "aegisvision",
		RefreshTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	called := 0
	h := authn(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))

	tok := signRS256(t, priv, kid, map[string]any{
		"iss":       "https://issuer.example",
		"aud":       "aegisvision",
		"sub":       "user-1",
		"tenant_id": "t-1",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest("GET", "/v1/pipelines", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if called != 1 {
		t.Fatalf("want handler called once, got %d", called)
	}
}

func TestAuthn_RejectsBadIssuer(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	kid := "k"
	jwks := rsaJWKS(t, priv, kid)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer ts.Close()

	authn, _ := middleware.Authn(middleware.AuthnConfig{
		JWKSURL: ts.URL, Issuer: "https://expected", Audience: "x",
		RefreshTimeout: time.Second,
	})
	h := authn(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler must not be called")
		_ = w
	}))

	tok := signRS256(t, priv, kid, map[string]any{
		"iss":       "https://attacker",
		"aud":       "x",
		"tenant_id": "t-1",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest("GET", "/v1/pipelines", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestAuthn_RejectsBadSignature(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	kid := "k"
	jwks := rsaJWKS(t, priv, kid)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer ts.Close()

	authn, _ := middleware.Authn(middleware.AuthnConfig{
		JWKSURL: ts.URL, Issuer: "https://issuer.example",
		RefreshTimeout: time.Second,
	})
	h := authn(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	tok := signRS256(t, other, kid, map[string]any{
		"iss":       "https://issuer.example",
		"tenant_id": "t-1",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest("GET", "/v1/pipelines", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestAuthn_RejectsExpired(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwks := rsaJWKS(t, priv, "k")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer ts.Close()
	authn, _ := middleware.Authn(middleware.AuthnConfig{JWKSURL: ts.URL, Issuer: "iss"})
	h := authn(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	tok := signRS256(t, priv, "k", map[string]any{
		"iss": "iss", "tenant_id": "t",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("want 401 (expired), got %d", rr.Code)
	}
}

func TestAuthn_RejectsMissingTenantClaim(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwks := rsaJWKS(t, priv, "k")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer ts.Close()
	authn, _ := middleware.Authn(middleware.AuthnConfig{JWKSURL: ts.URL, Issuer: "iss"})
	h := authn(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	tok := signRS256(t, priv, "k", map[string]any{
		"iss": "iss",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("want 401 (no tenant), got %d", rr.Code)
	}
}

func TestAuthn_RejectsMissingBearer(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwks := rsaJWKS(t, priv, "k")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer ts.Close()
	authn, _ := middleware.Authn(middleware.AuthnConfig{JWKSURL: ts.URL, Issuer: "iss"})
	h := authn(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/x", nil))
	if rr.Code != 401 {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

// _ uses context so unused-import linting doesn't trip on tighter builds.
var _ = context.Background

// _ for fmt errors-check.
func _unused(err error) string { return fmt.Sprintf("%v", err) }

// _ helps avoid the strings-unused warning across edits.
var _ = strings.Split
