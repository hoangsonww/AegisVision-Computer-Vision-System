package delivery_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aegisvision/services/notification-service/internal/delivery"
)

func TestDeliver_HappyPath(t *testing.T) {
	var got atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Add(1)
		if r.Header.Get("X-Aegis-Signature") == "" {
			t.Error("missing signature")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	c := delivery.NewClient(2 * time.Second)
	c.RetryBaseMS = 1
	out := c.Deliver(context.Background(), delivery.Endpoint{URL: ts.URL, Secret: "s", Tenant: "t-1"}, []byte(`{}`))
	if out.StatusCode != 200 || out.Err != nil {
		t.Fatalf("outcome: %+v", out)
	}
	if got.Load() != 1 {
		t.Fatalf("want 1 request, got %d", got.Load())
	}
}

func TestDeliver_RetriesThenSucceeds(t *testing.T) {
	var got atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := got.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	c := delivery.NewClient(2 * time.Second)
	c.RetryBaseMS = 1
	out := c.Deliver(context.Background(), delivery.Endpoint{URL: ts.URL, Secret: "s"}, []byte(`{}`))
	if out.StatusCode != 200 || out.Attempt != 3 {
		t.Fatalf("outcome: %+v", out)
	}
}

func TestDeliver_GivesUpAfterMaxAttempts(t *testing.T) {
	var got atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()
	c := delivery.NewClient(time.Second)
	c.RetryBaseMS = 1
	out := c.Deliver(context.Background(), delivery.Endpoint{URL: ts.URL, Secret: "s", MaxAttempts: 4}, []byte(`x`))
	if out.StatusCode != 502 || out.Attempt != 4 {
		t.Fatalf("outcome: %+v", out)
	}
	if got.Load() != 4 {
		t.Fatalf("server saw %d, want 4", got.Load())
	}
}

func TestSign_StableAndDifferentSecretChanges(t *testing.T) {
	a := delivery.Sign("k1", []byte("body"))
	b := delivery.Sign("k1", []byte("body"))
	c := delivery.Sign("k2", []byte("body"))
	if a != b {
		t.Fatal("Sign should be deterministic")
	}
	if a == c {
		t.Fatal("different secret must change signature")
	}
}
