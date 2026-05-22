// Package health exposes Kubernetes-style liveness and readiness probes.
//
// Liveness ("am I deadlocked?") is intentionally always green if the process
// is responsive. Readiness ("can I take traffic?") composes Check functions:
// any returning a non-nil error flips readiness to red and Kubernetes pulls
// the pod from the service endpoints.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Check returns nil when the dependency is healthy.
type Check func(context.Context) error

// Registry composes named readiness checks. Safe for concurrent use.
type Registry struct {
	mu     sync.RWMutex
	checks map[string]Check
}

func NewRegistry() *Registry {
	return &Registry{checks: make(map[string]Check)}
}

// Register adds or replaces a named readiness check.
func (r *Registry) Register(name string, c Check) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks[name] = c
}

// Deregister removes a check; useful for tests and for dependencies that come
// and go (e.g. an optional cache that the service can run without).
func (r *Registry) Deregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.checks, name)
}

type probeResult struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// LivenessHandler always returns 200 unless the process is so broken it
// cannot route a request — which is the right semantics for a kubelet probe.
func LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, probeResult{Status: "alive"})
	})
}

// ReadinessHandler evaluates every registered check with a 2-second timeout.
// Per-check failures are included in the response body so probes show up in
// pod events with a useful reason.
func (r *Registry) ReadinessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.RLock()
		snapshot := make(map[string]Check, len(r.checks))
		for k, v := range r.checks {
			snapshot[k] = v
		}
		r.mu.RUnlock()

		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()

		results := make(map[string]string, len(snapshot))
		allOK := true
		for name, c := range snapshot {
			if err := c(ctx); err != nil {
				results[name] = err.Error()
				allOK = false
			} else {
				results[name] = "ok"
			}
		}

		if allOK {
			writeJSON(w, http.StatusOK, probeResult{Status: "ready", Checks: results})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, probeResult{Status: "not ready", Checks: results})
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
