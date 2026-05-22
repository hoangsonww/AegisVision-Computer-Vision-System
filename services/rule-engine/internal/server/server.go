// Package server hosts rule-engine's diagnostic HTTP surface. The
// hot path is the bus loop in main.go; this server exposes:
//
//   GET  /v1/rules           — list the loaded rules
//   POST /v1/rules:reload    — reload rules from the configured source
//   POST /v1/rules:eval      — dry-run a detection against the rule set
//
// Operator-only — no tenant filtering applied; in production the
// API gateway is the right place to enforce per-tenant view of rules.
package server

import (
	"encoding/json"
	"net/http"
	"sync"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"

	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/rule-engine/internal/engine"
)

// Mux returns the diagnostic routes against an Engine that the caller
// (main.go) can swap atomically when rules reload.
type State struct {
	mu     sync.RWMutex
	engine *engine.Engine
	rules  []*engine.Rule
}

func NewState(e *engine.Engine, rules []*engine.Rule) *State {
	return &State{engine: e, rules: rules}
}

func (s *State) Swap(e *engine.Engine, rules []*engine.Rule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.engine = e
	s.rules = rules
}

func (s *State) Engine() *engine.Engine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.engine
}

func (s *State) Rules() []*engine.Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*engine.Rule, len(s.rules))
	copy(out, s.rules)
	return out
}

// Mux builds the diagnostic routes.
func Mux(state *State, reload func() error) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /v1/rules", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": state.Rules()})
	}))

	mux.Handle("POST /v1/rules:reload", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reload == nil {
			problem.BadRequest("reload not configured").Write(w)
			return
		}
		if err := reload(); err != nil {
			problem.Internal(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /v1/rules:eval", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var d dataplanev1.Detection
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		events := state.Engine().Apply(&d)
		writeJSON(w, http.StatusOK, map[string]any{"events": events})
	}))

	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
