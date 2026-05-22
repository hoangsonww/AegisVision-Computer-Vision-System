// Package server hosts notification-service's HTTP surface — CRUD on
// webhook endpoints. The actual delivery loop lives in internal/delivery
// and is wired from main.go's bus subscription.
//
// Routes (kept as `/v1/webhooks` shape):
//   POST /v1/webhooks         — register a webhook (url + secret)
//   GET  /v1/webhooks         — list this tenant's webhooks
package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/notification-service/internal/store"
)

// Mux builds the notification REST API.
func Mux(st *store.Store) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/webhooks", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL    string `json:"url"`
			Secret string `json:"secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		if in.URL == "" || in.Secret == "" {
			problem.Unprocessable("url and secret are required").Write(w)
			return
		}
		ep := store.Endpoint{
			ID: "wh-" + newID(), TenantID: logging.TenantID(r.Context()),
			URL: in.URL, Secret: in.Secret, Enabled: true,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := st.CreateEndpoint(r.Context(), ep); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": ep.ID})
	}))

	mux.Handle("GET /v1/webhooks", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		eps, err := st.ListByTenant(r.Context(), tenant)
		if err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		// Strip secrets on the wire — callers must already know them.
		out := make([]map[string]any, 0, len(eps))
		for _, e := range eps {
			out = append(out, map[string]any{
				"id": e.ID, "url": e.URL, "enabled": e.Enabled,
				"created_at": e.CreatedAt, "updated_at": e.UpdatedAt,
			})
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

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
