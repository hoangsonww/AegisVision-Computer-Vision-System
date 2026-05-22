// Package server is the metering-service HTTP surface.
//
// Routes (kept as the established `/v1/metering` shape):
//   GET  /v1/metering           — series for ?metric=&from=&to=
//   POST /v1/metering:bump      — admin-only manual bump (rare)
//
// Counters are owned by the consumer (internal/consumer); this surface
// is read-mostly. Cross-tenant reads are denied.
package server

import (
	"encoding/json"
	"net/http"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/metering-service/internal/store"
)

// Mux builds the metering routes against the provided store.
func Mux(st *store.Store) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /v1/metering", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		metric := r.URL.Query().Get("metric")
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		if metric == "" || from == "" || to == "" {
			problem.BadRequest("metric, from, to are required").Write(w)
			return
		}
		series, err := st.Range(r.Context(), tenant, metric, from, to)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"metric": metric, "series": series})
	}))

	mux.Handle("POST /v1/metering:bump", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Aegis-Role") != "platform-admin" {
			problem.Forbidden("admin role required").Write(w)
			return
		}
		var body struct {
			Tenant string `json:"tenant"`
			Metric string `json:"metric"`
			Count  int64  `json:"count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Count <= 0 {
			problem.BadRequest("invalid JSON or count <= 0").Write(w)
			return
		}
		if body.Tenant == "" || body.Metric == "" {
			problem.BadRequest("tenant and metric required").Write(w)
			return
		}
		if err := st.Increment(r.Context(), body.Tenant, body.Metric, body.Count); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
