package jwks_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aegisvision/services/auth-proxy/internal/jwks"
)

const rsaDoc = `{
  "keys": [
    {
      "kty": "RSA",
      "kid": "rsa-2026-05",
      "use": "sig",
      "alg": "RS256",
      "n": "v8X4eJlfH6gM6lWBgnDfBI3-EHy4xPiOSczo3wEZoG3IIIQRhfNTr5xLkFvIaW0g6T4xY-cQjQK-W2j-3pV9aF1bJ_oCBKjWqkF8d9swZqRn-Lhmts3CXC5p79i5d1bvkRczGNQyN0Mzbi7lE3ZkmCMQ7E9YfABNb6DLN6tHrUgM4Q1xb3OvIuP9d5BIPi-_mhxN1FmI3sJyZdGdVcKgC2ENZ3HXJrEnQ2cYZSrnXNoBjqMI67XHr-J1-zDA0r6vNV3lXn4Wq3w5w9d_a4xMQ-V8YgEhUkSljZ2zPjp9CcEEC1IItkEXljkOLuwO2HrSXkmaSb1MQGVZj0qBVDpYnQ",
      "e": "AQAB"
    }
  ]
}`

const ecDoc = `{
  "keys": [
    {
      "kty": "EC",
      "kid": "ec-2026-05",
      "use": "sig",
      "alg": "ES256",
      "crv": "P-256",
      "x": "MKBCTNIcKUSDii11ySs3526iDZ8AiTo7Tu6KPAqv7D4",
      "y": "4Etl6SRW2YiLUrN5vfvVHuhp7x8PxltmWWlbbM4IFyM"
    }
  ]
}`

func TestCache_FetchAndGet_RSA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rsaDoc))
	}))
	defer srv.Close()
	c := jwks.New(srv.URL)
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	k, ok := c.Get(context.Background(), "rsa-2026-05")
	if !ok {
		t.Fatal("expected to find rsa-2026-05")
	}
	if _, isRSA := k.Pub.(*rsa.PublicKey); !isRSA {
		t.Fatalf("expected *rsa.PublicKey, got %T", k.Pub)
	}
	if k.Alg != "RS256" {
		t.Fatalf("alg: %s", k.Alg)
	}
}

func TestCache_FetchAndGet_ECDSA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(ecDoc))
	}))
	defer srv.Close()
	c := jwks.New(srv.URL)
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	k, ok := c.Get(context.Background(), "ec-2026-05")
	if !ok {
		t.Fatal("expected to find ec-2026-05")
	}
	if _, isEC := k.Pub.(*ecdsa.PublicKey); !isEC {
		t.Fatalf("expected *ecdsa.PublicKey, got %T", k.Pub)
	}
}

func TestCache_RejectsHMAC(t *testing.T) {
	doc := `{"keys":[{"kty":"oct","kid":"hmac","use":"sig","alg":"HS256","k":"AAAA"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(doc))
	}))
	defer srv.Close()
	c := jwks.New(srv.URL)
	if err := c.Start(context.Background()); err == nil {
		t.Fatal("expected refresh to fail (no usable keys)")
	}
}

func TestCache_MissingKidTriggersRefresh(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(rsaDoc))
	}))
	defer srv.Close()
	c := jwks.New(srv.URL)
	c.MinRefresh = 1 // basically allow immediate forced refresh
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get(context.Background(), "unknown-kid"); ok {
		t.Fatal("expected miss")
	}
	if hits < 2 {
		t.Fatalf("expected forced refresh on miss, got %d hits", hits)
	}
}
