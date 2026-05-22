// Package server is the cost-accounting REST surface.
//
// Routes:
//   POST /v1/costs                — batch ingest from the collector job
//   GET  /v1/costs                — total + breakdown for the caller's tenant
package server

import (
	"encoding/json"
	"net/http"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/cost-accounting/internal/store"
)

func Mux(st *store.Store) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/costs", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch []store.Line
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			problem.BadRequest("expected array of cost lines").Write(w)
			return
		}
		applied := 0
		for _, l := range batch {
			if l.TenantID == "" || l.Day == "" || l.LineItem == "" {
				continue
			}
			if err := st.Upsert(r.Context(), l); err != nil {
				continue
			}
			applied++
		}
		writeJSON(w, http.StatusOK, map[string]any{"applied": applied, "total": len(batch)})
	}))

	mux.Handle("GET /v1/costs", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		if tenant == "" || from == "" || to == "" {
			problem.BadRequest("tenant, from, to required").Write(w)
			return
		}
		total, err := st.TenantTotal(r.Context(), tenant, from, to)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		br, _ := st.Breakdown(r.Context(), tenant, from, to)
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant_id": tenant, "from": from, "to": to,
			"total_usd": total, "breakdown_usd": br,
		})
	}))

	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
