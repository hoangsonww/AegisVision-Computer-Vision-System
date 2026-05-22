// Package server is the gpu-scheduler HTTP surface.
//
// Routes match the established URL shape (consumed by inference-router
// and dataplane-runner):
//
//   POST /v1/gpus      — operator registers a GPU
//   POST /v1/place     — request a placement (returns 503 on no capacity)
//   POST /v1/release   — release a reservation by id
//   GET  /v1/state     — read-only ledger snapshot
package server

import (
	"encoding/json"
	"net/http"

	"github.com/aegisvision/pkg/platform/problem"

	"github.com/aegisvision/services/gpu-scheduler/internal/ledger"
	"github.com/aegisvision/services/gpu-scheduler/internal/service"

	"github.com/prometheus/client_golang/prometheus"
)

// Counters bundles the service-specific counters main.go owns; we
// accept them as a dependency so server.Mux remains pure HTTP wiring.
type Counters struct {
	Placement *prometheus.CounterVec // labels: outcome (ok|no_capacity|error)
	FreeVRAM  *prometheus.GaugeVec   // labels: gpu, sku — refreshed on /v1/state
}

func Mux(svc *service.Service, c Counters) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/gpus", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var g ledger.GPU
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		if err := svc.Ledger.RegisterGPU(g); err != nil {
			problem.Unprocessable(err.Error()).Write(w)
			return
		}
		writeJSON(w, http.StatusCreated, g)
	}))

	mux.Handle("POST /v1/place", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ledger.PlacementRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		out, err := svc.Place(r.Context(), req)
		if err != nil {
			if isNoCapacity(err) {
				if c.Placement != nil {
					c.Placement.WithLabelValues("no_capacity").Inc()
				}
				problem.ServiceUnavailable("no GPU capacity").Write(w)
				return
			}
			if c.Placement != nil {
				c.Placement.WithLabelValues("error").Inc()
			}
			problem.Unprocessable(err.Error()).Write(w)
			return
		}
		if c.Placement != nil {
			c.Placement.WithLabelValues("ok").Inc()
		}
		writeJSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /v1/release", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		_ = svc.Release(r.Context(), body.ID)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /v1/state", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snap := svc.Snapshot(r.Context())
		if c.FreeVRAM != nil {
			for _, g := range snap.GPUs {
				free, _ := svc.Ledger.FreeVRAM(g.ID)
				c.FreeVRAM.WithLabelValues(g.ID, g.SKU).Set(float64(free))
			}
		}
		writeJSON(w, http.StatusOK, snap)
	}))

	return mux
}

// isNoCapacity inspects an error for the ErrNoCapacity signature without
// requiring callers to re-wrap. We compare against the ledger sentinel.
func isNoCapacity(err error) bool {
	for err != nil {
		if err == ledger.ErrNoCapacity {
			return true
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
