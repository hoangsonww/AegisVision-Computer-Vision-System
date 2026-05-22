// Package server hosts the auth-proxy's HTTP surface. It is deliberately
// thin — most of the work happens in `pkg/platform/middleware.Authn`,
// which already implements JWT verification against a JWKS document; this
// package wires those building blocks into the `/v1/check` endpoint that
// Envoy ext_authz (and forward-auth gateways) call.
package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	"github.com/prometheus/client_golang/prometheus"
)

// signedIdentity is the payload we sign into X-Aegis-Identity.
type signedIdentity struct {
	Tenant    string `json:"tenant"`
	RequestID string `json:"request_id"`
	IssuedAt  int64  `json:"iat"`
}

// CheckHandler returns the /v1/check ext_authz endpoint.
//
// Behavior:
//   - On success → 200, X-Aegis-Identity + X-Aegis-Tenant headers set.
//   - No tenant → 401 application/problem+json.
//
// The signed identity is HMAC-SHA256 over the JSON body, formatted as
// "<base64(body)>.<base64url(mac)>" — downstream verifiers split on "."
// and re-compute the MAC with the same key (see VerifyIdentityHeader).
func CheckHandler(identityKey string, decisions *prometheus.CounterVec) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := logging.TenantID(r.Context())
		if tenant == "" {
			if decisions != nil {
				decisions.WithLabelValues("denied").Inc()
			}
			problem.Unauthorized("no tenant").Write(w)
			return
		}
		identity := signedIdentity{
			Tenant:    tenant,
			RequestID: logging.RequestID(r.Context()),
			IssuedAt:  time.Now().UTC().Unix(),
		}
		body, _ := json.Marshal(identity)
		sig := SignIdentity(identityKey, body)
		w.Header().Set("X-Aegis-Identity", base64.StdEncoding.EncodeToString(body)+"."+sig)
		w.Header().Set("X-Aegis-Tenant", tenant)
		if decisions != nil {
			decisions.WithLabelValues("ok").Inc()
		}
		w.WriteHeader(http.StatusOK)
	})
}

// SignIdentity HMACs the body with the shared identity key. Exposed so
// main.go (and downstream services) can share the same formula.
func SignIdentity(key string, body []byte) string {
	if key == "" {
		key = "dev-identity-key"
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyIdentityHeader is the downstream-side check: returns the tenant
// id if the X-Aegis-Identity header is well-formed AND signed by the
// same identity key. Returns ("", false) on any failure — callers
// should reject the request.
func VerifyIdentityHeader(key, header string) (string, bool) {
	if header == "" {
		return "", false
	}
	dot := -1
	for i := len(header) - 1; i >= 0; i-- {
		if header[i] == '.' {
			dot = i
			break
		}
	}
	if dot <= 0 || dot == len(header)-1 {
		return "", false
	}
	body64 := header[:dot]
	gotSig := header[dot+1:]
	body, err := base64.StdEncoding.DecodeString(body64)
	if err != nil {
		return "", false
	}
	want := SignIdentity(key, body)
	if !hmac.Equal([]byte(want), []byte(gotSig)) {
		return "", false
	}
	var id signedIdentity
	if err := json.Unmarshal(body, &id); err != nil {
		return "", false
	}
	return id.Tenant, true
}

// Chain composes middleware in declared order — first listed wraps
// outermost.
func Chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}
