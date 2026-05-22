// Package server is the tenant-service HTTP surface. The full main.go
// route table is mirrored here so callers (and tests) can mount the
// routes without going through the daemon.
package server

import (
	"encoding/json"
	"net/http"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/tenant-service/internal/service"
	"github.com/aegisvision/services/tenant-service/internal/store"
)

func Mux(svc *service.Service) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/tenants", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var t store.Tenant
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		out, err := svc.CreateTenant(r.Context(), t)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}))

	mux.Handle("GET /v1/tenants", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, err := svc.ListTenants(r.Context())
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}))

	mux.Handle("GET /v1/tenants/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t, err := svc.GetTenant(r.Context(), r.PathValue("id"))
		if err != nil {
			problem.NotFound("tenant not found").Write(w)
			return
		}
		writeJSON(w, http.StatusOK, t)
	}))

	mux.Handle("DELETE /v1/tenants/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := svc.SoftDelete(r.Context(), r.PathValue("id")); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /v1/tenants/{id}/quotas", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out, err := svc.ListQuotas(r.Context(), r.PathValue("id"))
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}))

	mux.Handle("PUT /v1/tenants/{id}/quotas/{metric}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q store.Quota
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		q.TenantID = r.PathValue("id")
		q.Metric = r.PathValue("metric")
		if err := svc.UpsertQuota(r.Context(), q); err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, q)
	}))

	mux.Handle("POST /v1/dsar", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var d store.DSAR
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		d.RequestedBy = r.Header.Get("X-Aegis-Subject")
		if d.RequestedBy == "" {
			// Backward-compat with the pre-auth-proxy header name.
			d.RequestedBy = r.Header.Get("X-User")
		}
		if d.TenantID == "" {
			d.TenantID = logging.TenantID(r.Context())
		}
		out, err := svc.FileDSAR(r.Context(), d)
		if err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusAccepted, out)
	}))

	mux.Handle("GET /v1/dsar", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := r.URL.Query().Get("tenant_id")
		if tenant == "" {
			tenant = logging.TenantID(r.Context())
		}
		out, err := svc.ListDSAR(r.Context(), tenant)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}))

	mux.Handle("POST /v1/dsar/{id}/complete", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ArtifactURI string `json:"artifact_uri"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := svc.CompleteDSAR(r.Context(), r.PathValue("id"), body.ArtifactURI); err != nil {
			problem.NotFound("dsar not found").Write(w)
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
