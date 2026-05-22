package prom_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aegisvision/services/slo-watchdog/internal/prom"
)

func TestClient_Query_Vector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"foo":"bar"},"value":[1747000000,"0.997"]}]}}`))
	}))
	defer srv.Close()
	c := prom.New(srv.URL)
	v, err := c.Query(context.Background(), "up{}", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if v < 0.99 || v > 1.0 {
		t.Fatalf("v: %f", v)
	}
}

func TestClient_Query_EmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()
	c := prom.New(srv.URL)
	if _, err := c.Query(context.Background(), "x", time.Now()); err == nil {
		t.Fatal("expected error on empty result")
	}
}

func TestClient_Query_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	c := prom.New(srv.URL)
	if _, err := c.Query(context.Background(), "x", time.Now()); err == nil {
		t.Fatal("expected error on 400")
	}
}
