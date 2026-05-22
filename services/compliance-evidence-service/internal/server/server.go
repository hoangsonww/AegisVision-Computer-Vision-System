// Package server hosts compliance-evidence-service's HTTP surface.
//
// Routes (all tenant-scoped via X-Aegis-Tenant header, all read-only):
//
//   GET  /v1/evidence?control=CC6.1&from=...&to=...&limit=...
//                              — JSON envelope { items: [...] }
//   GET  /v1/evidence.csv?...  — same query, CSV download (auditors prefer)
//   GET  /v1/evidence.jsonl?...— streaming JSON-Lines (for very large reports)
//   GET  /v1/controls          — list controls supported by the configured collectors
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/compliance-evidence-service/internal/evidence"
)

func Mux(svc *evidence.Service) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /v1/evidence", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := requestFromURL(r)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		recs, err := svc.Collect(r.Context(), req)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": recs})
	}))

	mux.Handle("GET /v1/evidence.csv", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := requestFromURL(r)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		recs, err := svc.Collect(r.Context(), req)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		fname := fmt.Sprintf("evidence-%s-%s.csv",
			req.TenantID, time.Now().UTC().Format("20060102-150405"))
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+fname+"\"")
		_ = evidence.WriteCSV(w, recs)
	}))

	mux.Handle("GET /v1/evidence.jsonl", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := requestFromURL(r)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		recs, err := svc.Collect(r.Context(), req)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		w.Header().Set("Content-Type", "application/jsonl")
		_ = evidence.WriteJSON(w, recs)
	}))

	mux.Handle("GET /v1/controls", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		set := map[evidence.Control]bool{}
		for _, c := range svc.Collectors {
			for _, k := range c.Controls() {
				set[k] = true
			}
		}
		out := make([]evidence.Control, 0, len(set))
		for k := range set {
			out = append(out, k)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": out})
	}))

	return mux
}

func requestFromURL(r *http.Request) (evidence.Request, error) {
	tenant := logging.TenantID(r.Context())
	if tenant == "" {
		return evidence.Request{}, fmt.Errorf("tenant required (X-Aegis-Tenant)")
	}
	req := evidence.Request{
		TenantID: tenant,
		Control:  evidence.Control(r.URL.Query().Get("control")),
	}
	if s := r.URL.Query().Get("from"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return req, fmt.Errorf("bad from=: %w", err)
		}
		req.From = t
	}
	if s := r.URL.Query().Get("to"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return req, fmt.Errorf("bad to=: %w", err)
		}
		req.To = t
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			return req, fmt.Errorf("bad limit=: %w", err)
		}
		req.Limit = n
	}
	return req, nil
}
