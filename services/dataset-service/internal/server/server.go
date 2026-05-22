// Package server is the dataset-service HTTP surface.
//
// Routes:
//   POST /v1/datasets               — create dataset
//   GET  /v1/datasets               — list datasets (current tenant)
//   GET  /v1/datasets/{id}          — fetch one
//   POST /v1/datasets/{id}/versions — create a new version
//   GET  /v1/datasets/{id}/versions — list versions
package server

import (
	"encoding/json"
	"net/http"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/dataset-service/internal/service"
	"github.com/aegisvision/services/dataset-service/internal/store"
)

func Mux(svc *service.Service) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/datasets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var d store.Dataset
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		d.TenantID = logging.TenantID(r.Context())
		out, err := svc.CreateDataset(r.Context(), d)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}))

	mux.Handle("GET /v1/datasets", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, err := svc.ListDatasets(r.Context(), logging.TenantID(r.Context()))
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}))

	mux.Handle("GET /v1/datasets/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, err := svc.GetDataset(r.Context(), r.PathValue("id"))
		if err != nil {
			problem.NotFound("dataset not found").Write(w)
			return
		}
		if t := logging.TenantID(r.Context()); t != "" && t != d.TenantID {
			problem.Forbidden("cross-tenant read denied").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}))

	mux.Handle("POST /v1/datasets/{id}/versions", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v store.Version
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		v.DatasetID = r.PathValue("id")
		out, err := svc.CreateVersion(r.Context(), v)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}))

	mux.Handle("GET /v1/datasets/{id}/versions", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, err := svc.ListVersions(r.Context(), r.PathValue("id"))
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
