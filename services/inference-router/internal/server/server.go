// Package server is the inference-router HTTP surface.
//
// Routes:
//   POST /v1/infer              — run inference
//   POST /v1/models:warmup      — pre-warm a (model, version) pair (idempotent)
//   GET  /v1/models             — list known models seen since startup
package server

import (
	"encoding/json"
	"net/http"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"

	"github.com/aegisvision/services/inference-router/internal/service"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Counters bundles the service-specific counter owned by main.go.
type Counters struct {
	// Invocations: labels = model, version, outcome (ok|error).
	Invocations *prometheus.CounterVec
}

// inferResponse is the JSON shape the inference-router emits.
type inferResponse struct {
	Detections []*dataplanev1.Detection `json:"detections"`
}

func Mux(svc *service.Service, c Counters) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /v1/infer", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req service.InferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			problem.BadRequest("invalid JSON body").Write(w)
			return
		}
		// Tenant from identity overrides any payload value. The frame URN
		// + frame descriptor are forwarded to the detector + the bus
		// events.
		if t := logging.TenantID(r.Context()); t != "" {
			req.TenantID = t
		}
		req.Frame = &dataplanev1.FrameDescriptor{
			StreamId:    req.StreamID,
			PipelineId:  req.PipelineID,
			FrameSeq:    req.FrameSeq,
			CaptureTime: timestamppb.New(req.CaptureTime.UTC()),
		}
		out, err := svc.Infer(r.Context(), req)
		if err != nil {
			if c.Invocations != nil {
				c.Invocations.WithLabelValues(req.Model, req.Version, "error").Inc()
			}
			problem.ServiceUnavailable("inference failed: " + err.Error()).Write(w)
			return
		}
		if c.Invocations != nil {
			c.Invocations.WithLabelValues(req.Model, req.Version, "ok").Inc()
		}
		writeJSON(w, http.StatusOK, inferResponse{Detections: out})
	}))

	mux.Handle("POST /v1/models:warmup", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model   string `json:"model"`
			Version string `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			problem.BadRequest("invalid JSON").Write(w)
			return
		}
		if err := svc.Warmup(r.Context(), body.Model, body.Version); err != nil {
			problem.BadRequest(err.Error()).Write(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /v1/models", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": svc.Models()})
	}))

	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
