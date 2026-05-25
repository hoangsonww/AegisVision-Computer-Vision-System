// Package service wraps the detector with routing logic: pick a model
// for the request, validate, run inference, record telemetry. This is
// the surface the HTTP layer + (future) gRPC layer both consume.
//
// Why this exists separately from internal/detector: detector is the
// raw Triton client. service is the policy layer — model resolution,
// per-tenant guardrails, request validation, and the synthetic dev
// fallback when no Triton URL is configured.
//
// Bus integration: on every successful inference the service publishes
// two events:
//
//   - `inference.completed.v1` — consumed by metering-service (billing)
//   - `inference.baseline.v1`  — consumed by shadow-inference-service
//                                (canary regression detector). Carries
//                                only the URN + predicted class — no
//                                frame bytes (ADR-0008).
//
// The Publisher is optional; when nil (dev / single-binary tests) the
// publish path is a silent no-op.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"

	"github.com/aegisvision/pkg/bus"

	"github.com/aegisvision/services/inference-router/internal/detector"
)

// Detector is the contract the service consumes — both Triton clients
// and the synthetic fallback implement this.
type Detector interface {
	Detect(ctx context.Context, frame *dataplanev1.FrameDescriptor, payload []byte) ([]*dataplanev1.Detection, error)
}

// Service is the inference router.
type Service struct {
	// TritonBase is the base URL for the configured Triton inference
	// server. When empty, the service uses the synthetic fallback —
	// safe for dev/test, refuses to start in prod (main.go gates).
	TritonBase string

	// TritonTimeoutSeconds caps a single Triton call (incl. retries).
	// Zero = use the detector default (5s).
	TritonTimeoutSeconds int

	// TritonMaxRetries on transient (5xx + network) failures.
	// Zero = use the detector default (2).
	TritonMaxRetries int

	// Bus is the optional publisher for inference.* events. Nil means
	// no events published (dev / unit-test path).
	Bus bus.Publisher

	mu    sync.RWMutex
	known map[string]struct{}
	Now   func() time.Time
}

// New constructs a Service with an optional bus publisher.
func New(tritonBase string, pub bus.Publisher) *Service {
	return &Service{
		TritonBase: tritonBase,
		Bus:        pub,
		known:      map[string]struct{}{},
		Now:        func() time.Time { return time.Now().UTC() },
	}
}

// InferRequest is the platform-internal shape of an inference call.
type InferRequest struct {
	TenantID    string                       `json:"tenant_id"`
	Model       string                       `json:"model"`
	Version     string                       `json:"version"`
	StreamID    string                       `json:"stream_id"`
	PipelineID  string                       `json:"pipeline_id"`
	FrameSeq    int64                        `json:"frame_seq"`
	FrameURN    string                       `json:"frame_urn"`
	CaptureTime time.Time                    `json:"capture_time"`
	Payload     []byte                       `json:"payload"`
	Frame       *dataplanev1.FrameDescriptor `json:"-"`
}

// Validate checks required fields.
func (r *InferRequest) Validate() error {
	if r.Model == "" || r.Version == "" {
		return errors.New("model and version required")
	}
	return nil
}

// Infer runs the model. Records the (model, version) in the known set,
// publishes inference.completed.v1 (for metering) and
// inference.baseline.v1 (for shadow-inference canary comparison).
//
// When the Triton client surfaces a response-cache hit, the emitted
// inference.completed.v1 carries cache_hit=true so metering can credit
// cached calls appropriately.
func (s *Service) Infer(ctx context.Context, req InferRequest) ([]*dataplanev1.Detection, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var (
		det       Detector
		tritonCli *detector.TritonClient
	)
	if s.TritonBase != "" {
		opts := detector.Options{}
		if s.TritonTimeoutSeconds > 0 {
			opts.PerRequestTimeout = time.Duration(s.TritonTimeoutSeconds) * time.Second
		}
		if s.TritonMaxRetries > 0 {
			opts.MaxRetries = s.TritonMaxRetries
		}
		tritonCli = detector.NewTritonWithOptions(s.TritonBase, req.Model, req.Version, opts)
		det = tritonCli
	} else {
		det = &syntheticDetector{model: req.Model, version: req.Version}
	}
	s.mu.Lock()
	s.known[req.Model+"@"+req.Version] = struct{}{}
	s.mu.Unlock()
	start := s.Now()
	out, err := det.Detect(ctx, req.Frame, req.Payload)
	if err != nil {
		return nil, err
	}
	var cacheHit bool
	if tritonCli != nil {
		cacheHit = tritonCli.LastStats().CacheHit
	}
	s.emit(ctx, req, out, s.Now().Sub(start), cacheHit)
	return out, nil
}

// emit publishes the two bus events. Errors are swallowed because the
// inference RPC has already succeeded — we don't want a bus blip to
// take down the caller's request.
func (s *Service) emit(ctx context.Context, req InferRequest, dets []*dataplanev1.Detection, latency time.Duration, cacheHit bool) {
	if s.Bus == nil {
		return
	}
	// inference.completed.v1 — metering bills on this count.
	completed, _ := json.Marshal(map[string]any{
		"tenant_id":    req.TenantID,
		"model_id":     req.Model,
		"version":      req.Version,
		"stream_id":    req.StreamID,
		"pipeline_id":  req.PipelineID,
		"frame_seq":    req.FrameSeq,
		"detections":   len(dets),
		"latency_ms":   latency.Milliseconds(),
		"cache_hit":    cacheHit,
		"completed_at": s.Now().Format(time.RFC3339Nano),
	})
	_ = s.Bus.Publish(ctx, bus.Message{
		Subject: "inference.completed.v1",
		Data:    completed,
		Headers: map[string]string{
			"tenant_id": req.TenantID,
			"model_id":  req.Model,
		},
	})
	// inference.baseline.v1 — shadow-inference compares against this.
	// Carries only the URN + top-1 prediction (no frame bytes, ADR-0008).
	var predicted string
	var confidence float64
	if len(dets) > 0 {
		predicted = dets[0].ClassLabel
		confidence = float64(dets[0].Score)
	}
	baseline, _ := json.Marshal(map[string]any{
		"tenant_id":  req.TenantID,
		"stream_id":  req.StreamID,
		"frame_urn":  req.FrameURN,
		"model_id":   req.Model,
		"predicted":  predicted,
		"confidence": confidence,
		"latency_ms": latency.Milliseconds(),
	})
	_ = s.Bus.Publish(ctx, bus.Message{
		Subject: "inference.baseline.v1",
		Data:    baseline,
		Headers: map[string]string{"tenant_id": req.TenantID, "stream_id": req.StreamID},
	})
}

// Models returns the (model, version) pairs the service has been asked
// for since startup.
func (s *Service) Models() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.known))
	for k := range s.known {
		out = append(out, k)
	}
	return out
}

// Warmup pre-registers a (model, version) so prefetch-service can drive
// the router.
func (s *Service) Warmup(_ context.Context, model, version string) error {
	if model == "" || version == "" {
		return errors.New("model and version required")
	}
	s.mu.Lock()
	s.known[model+"@"+version] = struct{}{}
	s.mu.Unlock()
	return nil
}

// syntheticDetector is the dev fallback. Mirrors operators.SyntheticDetector
// at a JSON boundary.
type syntheticDetector struct{ model, version string }

func (s *syntheticDetector) Detect(_ context.Context, frame *dataplanev1.FrameDescriptor, _ []byte) ([]*dataplanev1.Detection, error) {
	return []*dataplanev1.Detection{{
		StreamId: frame.GetStreamId(), PipelineId: frame.GetPipelineId(),
		FrameSeq: frame.GetFrameSeq(), CaptureTime: frame.GetCaptureTime(),
		ModelId: s.model, ModelVersionId: s.version,
		ClassLabel: "person", Score: 0.9,
		Bbox: &dataplanev1.BoundingBox{X: 0.5, Y: 0.5, W: 0.1, H: 0.1},
	}}, nil
}
