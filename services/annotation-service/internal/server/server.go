// Package server is the HTTP layer of annotation-service. It exports
// Mux() so main.go (and tests) can mount the routes without going
// through the full daemon lifecycle.
//
// Routes:
//   POST /v1/annotations               — create
//   POST /v1/annotations/{id}:review   — review (approve/reject)
//   GET  /v1/annotations               — list by ?dataset_id=&item_id=
package server

import (
	"encoding/json"
	"net/http"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/annotation-service/internal/service"
	"github.com/aegisvision/services/annotation-service/internal/store"
)

// Mux returns the routed handler. Callers wrap with their middleware
// stack (recovery, request-id, tenant, RED metrics).
func Mux(svc *service.Service) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/annotations", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var a store.Annotation
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		a.TenantID = logging.TenantID(r.Context())
		out, err := svc.Create(r.Context(), a)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}))

	mux.Handle("POST /v1/annotations/{id}/review", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Status   string `json:"status"`
			Reviewer string `json:"reviewer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		if err := svc.Review(r.Context(), r.PathValue("id"), body.Status, body.Reviewer); err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /v1/annotations", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dataset := r.URL.Query().Get("dataset_id")
		item := r.URL.Query().Get("item_id")
		if dataset == "" || item == "" {
			problem.BadRequest("dataset_id and item_id required").Write(w)
			return
		}
		out, err := svc.ListByItem(r.Context(), dataset, item)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}))

	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
