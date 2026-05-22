package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aegisvision/services/edge-gateway/internal/server"
)

type stubFwd struct {
	forwarded []forwarded
	stats     server.Stats
	flushed   int
	flushErr  error
}

type forwarded struct {
	subject string
	data    []byte
}

func (s *stubFwd) Forward(_ context.Context, subj string, data []byte) (uint64, error) {
	s.forwarded = append(s.forwarded, forwarded{subject: subj, data: append([]byte(nil), data...)})
	return uint64(len(s.forwarded)), nil
}
func (s *stubFwd) Stats() server.Stats { return s.stats }
func (s *stubFwd) FlushNow(_ context.Context) (int, error) {
	if s.flushErr != nil {
		return 0, s.flushErr
	}
	return s.flushed, nil
}

func TestServer_ForwardAccepts(t *testing.T) {
	f := &stubFwd{}
	srv := httptest.NewServer(server.Mux(f))
	defer srv.Close()
	body, _ := json.Marshal(map[string]any{"subject": "events.v1", "data": json.RawMessage(`{"x":1}`)})
	resp, err := http.Post(srv.URL+"/v1/forward", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if len(f.forwarded) != 1 || f.forwarded[0].subject != "events.v1" {
		t.Fatalf("forwarded: %+v", f.forwarded)
	}
}

func TestServer_ForwardRejectsMissingSubject(t *testing.T) {
	srv := httptest.NewServer(server.Mux(&stubFwd{}))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/v1/forward", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestServer_BatchCountsAccepted(t *testing.T) {
	f := &stubFwd{}
	srv := httptest.NewServer(server.Mux(f))
	defer srv.Close()
	body, _ := json.Marshal([]map[string]any{
		{"subject": "events.v1", "data": json.RawMessage(`{}`)},
		{"subject": "events.v1", "data": json.RawMessage(`{}`)},
		{"subject": "", "data": json.RawMessage(`{}`)}, // dropped
	})
	resp, err := http.Post(srv.URL+"/v1/forward:batch", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]int
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["accepted"] != 2 || out["total"] != 3 {
		t.Fatalf("counts: %+v", out)
	}
}

func TestServer_StatsReflectsForwarder(t *testing.T) {
	f := &stubFwd{stats: server.Stats{Items: 42, Bytes: 999, OldestAgeSeconds: 60}}
	srv := httptest.NewServer(server.Mux(f))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/queue/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if int(out["items"].(float64)) != 42 {
		t.Fatalf("items: %v", out["items"])
	}
}

func TestServer_FlushRequiresIdentity(t *testing.T) {
	srv := httptest.NewServer(server.Mux(&stubFwd{}))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/v1/queue:flush", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestServer_FlushErrorPropagates(t *testing.T) {
	f := &stubFwd{flushErr: errors.New("disk i/o")}
	srv := httptest.NewServer(server.Mux(f))
	defer srv.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/v1/queue:flush", nil)
	req.Header.Set("X-Aegis-Subject", "oncall@aegis")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}
