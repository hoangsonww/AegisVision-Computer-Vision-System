// Package server is the training-orchestrator REST surface.
//
// Routes (kept as `/v1/training-jobs` for backwards compatibility with
// callers that integrated against the pre-refactor URL shape):
//
//   POST   /v1/training-jobs               — submit a new job
//   GET    /v1/training-jobs               — list (current tenant)
//   GET    /v1/training-jobs/{id}          — fetch one
//   PATCH  /v1/training-jobs/{id}          — update state/progress (runner reports back)
//   POST   /v1/training-jobs/{id}:cancel   — cancel a queued/running job
package server

import (
	"encoding/json"
	"net/http"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/training-orchestrator/internal/service"
	"github.com/aegisvision/services/training-orchestrator/internal/store"
)

func Mux(svc *service.Service) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/training-jobs", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var j store.Job
		if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		j.TenantID = logging.TenantID(r.Context())
		out, err := svc.Submit(r.Context(), j)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}))

	mux.Handle("GET /v1/training-jobs", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, err := svc.ListByTenant(r.Context(), logging.TenantID(r.Context()))
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}))

	mux.Handle("GET /v1/training-jobs/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		j, err := svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			problem.NotFound("job not found").Write(w)
			return
		}
		if t := logging.TenantID(r.Context()); t != "" && t != j.TenantID {
			problem.Forbidden("cross-tenant read denied").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, j)
	}))

	mux.Handle("PATCH /v1/training-jobs/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			State, MetricsJSON, ArtifactURI, LastError string
			EpochsDone                                 int
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		if err := svc.UpdateProgress(r.Context(), r.PathValue("id"), in.State, in.EpochsDone, in.MetricsJSON, in.ArtifactURI, in.LastError); err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /v1/training-jobs/{id}/cancel", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Cancel(r.Context(), r.PathValue("id")); err != nil {
			problem.BadRequest(err.Error()).Write(w)
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
