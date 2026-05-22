// Package server is audit-service's HTTP surface. Read-only — the
// canonical writer is the bus consumer (internal/consumer); the only
// POST here is an emergency-ingest endpoint guarded by the admin role.
//
// Routes (kept as `/v1/audit` shape):
//   GET  /v1/audit              — paginated list, tenant-scoped
//   POST /v1/audit              — admin-only direct ingest (rare)
package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/audit-service/internal/store"
)

func Mux(st *store.Store) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /v1/audit", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		outcome := r.URL.Query().Get("outcome")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		recs, err := st.List(r.Context(), tenant, outcome, limit)
		if err != nil {
			problem.Internal("query failed").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": recs})
	}))

	mux.Handle("POST /v1/audit", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Aegis-Role") != "platform-admin" {
			problem.Forbidden("admin role required").Write(w)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			problem.BadRequest("could not read body").Write(w)
			return
		}
		if err := st.AppendJSON(r.Context(), body); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))

	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
