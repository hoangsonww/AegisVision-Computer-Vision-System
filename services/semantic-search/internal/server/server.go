// Package server is the semantic-search HTTP surface.
//
// Routes (kept as the established `/v1/vectors` shape):
//   POST   /v1/vectors          — upsert a vector
//   DELETE /v1/vectors/{id}     — delete
//   POST   /v1/search           — k-NN search
//   GET    /v1/index/stats      — size
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/semantic-search/internal/index"
	"github.com/aegisvision/services/semantic-search/internal/service"

	"github.com/prometheus/client_golang/prometheus"
)

// Gauges bundles the index-size gauge owned by main.go.
type Gauges struct {
	IndexSize prometheus.Gauge
}

func Mux(svc *service.Service, g Gauges) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/vectors", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var it index.Item
		if err := json.NewDecoder(r.Body).Decode(&it); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		it.TenantID = logging.TenantID(r.Context())
		if err := svc.Upsert(r.Context(), it); err != nil {
			problem.Unprocessable(err.Error()).Write(w)
			return
		}
		if g.IndexSize != nil {
			g.IndexSize.Set(float64(svc.Size()))
		}
		w.WriteHeader(http.StatusCreated)
	}))

	mux.Handle("DELETE /v1/vectors/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		svc.Delete(r.Context(), r.PathValue("id"))
		if g.IndexSize != nil {
			g.IndexSize.Set(float64(svc.Size()))
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /v1/search", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Vector []float32         `json:"vector"`
			K      int               `json:"k"`
			Filter map[string]string `json:"filter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		k := in.K
		if k <= 0 {
			if s := r.URL.Query().Get("k"); s != "" {
				k, _ = strconv.Atoi(s)
			}
		}
		hits, err := svc.Search(r.Context(), logging.TenantID(r.Context()), in.Vector, k, in.Filter)
		if err != nil {
			problem.Unprocessable(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
	}))

	mux.Handle("GET /v1/index/stats", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"size": svc.Size()})
	}))

	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
