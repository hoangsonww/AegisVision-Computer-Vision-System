// Package server hosts the media-service HTTP surface.
//
// Routes:
//   POST /v1/media                — create a media record
//   GET  /v1/media/{id}           — fetch one
//   POST /v1/media/{id}:promote   — copy-on-promote (Matroid scar)
//   GET  /v1/media:due-retention  — list rows due for retention sweep
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/media-service/internal/service"
	"github.com/aegisvision/services/media-service/internal/store"
)

func Mux(svc *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/media", create(svc))
	mux.Handle("GET /v1/media/{id}", get(svc))
	mux.Handle("POST /v1/media/{id}/promote", promote(svc))
	mux.Handle("GET /v1/media/due-retention", dueRetention(svc))
	return mux
}

func create(svc *service.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m store.Media
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		m.TenantID = logging.TenantID(r.Context())
		out, err := svc.Create(r.Context(), m)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	})
}

func get(svc *service.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m, err := svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			problem.NotFound("media not found").Write(w)
			return
		}
		if tenant := logging.TenantID(r.Context()); tenant != "" && tenant != m.TenantID {
			problem.Forbidden("cross-tenant read denied").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, m)
	})
}

func promote(svc *service.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			NewURI string `json:"new_uri"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		if err := svc.PromoteCopy(r.Context(), r.PathValue("id"), body.NewURI); err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func dueRetention(svc *service.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
				limit = n
			}
		}
		out, err := svc.DueForRetention(r.Context(), limit)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
