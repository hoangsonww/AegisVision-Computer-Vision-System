// The detector operator runs an inference Detector against the claim-check
// payload referenced by each frame, attaches detections to the envelope,
// and forwards. The Detector interface is the seam where a real Triton
// client (production), a DeepStream pipeline, or a custom backend plugs
// in; the walking skeleton ships SyntheticDetector which reads the JSON
// payload back from the ring and produces a single detection per frame
// at the encoded coordinates.
package operators

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aegisvision/pkg/dataplane"
	"github.com/aegisvision/pkg/dataplane/claimcheck"
	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

// Detector is the abstract interface any model serving layer satisfies.
// The NVIDIA Triton client in services/inference-router/internal/detector
// implements this against a real Triton Inference Server;
// SyntheticDetector implements it deterministically for tests and the
// walking-skeleton demo.
type Detector interface {
	// Detect produces zero or more Detections for the supplied frame payload.
	// The payload bytes are whatever shape the upstream operator agreed with
	// the detector — pixels for a real model, JSON for the skeleton.
	Detect(ctx context.Context, frame *dataplanev1.FrameDescriptor, payload []byte) ([]*dataplanev1.Detection, error)
	// Name identifies the underlying model version for metrics + audit.
	ModelID() string
	ModelVersionID() string
}

// DetectConfig wires a Detector + Ring into the operator.
type DetectConfig struct {
	Detector Detector
	Ring     *claimcheck.Ring
	Logger   *slog.Logger
}

type Detect struct {
	cfg DetectConfig
}

func NewDetect(cfg DetectConfig) *Detect { return &Detect{cfg: cfg} }

func (d *Detect) Name() string { return "detect" }

func (d *Detect) Run(ctx context.Context, in <-chan *dataplane.FrameEnvelope, out chan<- *dataplane.FrameEnvelope) error {
	if d.cfg.Detector == nil || d.cfg.Ring == nil {
		return fmt.Errorf("detect: detector and ring are required")
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case env, ok := <-in:
			if !ok {
				return dataplane.ErrUpstreamClosed
			}
			loc, err := claimcheck.ParseLocator(env.Descriptor.GetPayload().GetLocator())
			if err != nil {
				if d.cfg.Logger != nil {
					d.cfg.Logger.Warn("invalid locator", slog.String("err", err.Error()))
				}
				continue
			}
			payload, err := d.cfg.Ring.Get(loc)
			if err != nil {
				if d.cfg.Logger != nil {
					d.cfg.Logger.Warn("payload fetch failed",
						slog.Int64("frame_seq", env.Descriptor.GetFrameSeq()),
						slog.String("err", err.Error()))
				}
				continue
			}
			dets, err := d.cfg.Detector.Detect(ctx, env.Descriptor, payload)
			if err != nil {
				if d.cfg.Logger != nil {
					d.cfg.Logger.Warn("detector failed", slog.String("err", err.Error()))
				}
				continue
			}
			env.Detections = append(env.Detections, dets...)
			select {
			case <-ctx.Done():
				return nil
			case out <- env:
			}
		}
	}
}

// SyntheticDetector reads the JSON payload produced by Ingest and turns it
// back into a single Detection at the encoded coordinates. Deterministic;
// useful for testing the full chain.
type SyntheticDetector struct {
	ModelID_, ModelVersionID_ string
}

func NewSyntheticDetector() *SyntheticDetector {
	return &SyntheticDetector{
		ModelID_:        "synthetic-detector",
		ModelVersionID_: "v1",
	}
}

func (s *SyntheticDetector) ModelID() string        { return s.ModelID_ }
func (s *SyntheticDetector) ModelVersionID() string { return s.ModelVersionID_ }

func (s *SyntheticDetector) Detect(_ context.Context, frame *dataplanev1.FrameDescriptor, payload []byte) ([]*dataplanev1.Detection, error) {
	var t SyntheticTarget
	if err := json.Unmarshal(payload, &t); err != nil {
		return nil, fmt.Errorf("synthetic: %w", err)
	}
	det := &dataplanev1.Detection{
		TenantId:       extractTenant(frame),
		PipelineId:     frame.GetPipelineId(),
		StreamId:       frame.GetStreamId(),
		FrameSeq:       frame.GetFrameSeq(),
		CaptureTime:    frame.GetCaptureTime(),
		ModelId:        s.ModelID_,
		ModelVersionId: s.ModelVersionID_,
		ClassLabel:     t.Class,
		Score:          t.Score,
		Bbox: &dataplanev1.BoundingBox{
			X: t.X, Y: t.Y, W: t.W, H: t.H,
		},
	}
	return []*dataplanev1.Detection{det}, nil
}

// extractTenant resolves the owning tenant from a frame; in the data plane
// the tenant is propagated as a header in the FrameDescriptor's provenance
// attestations or via a per-stream lookup cache. The skeleton encodes it
// as an attestation entry to keep wiring honest.
func extractTenant(frame *dataplanev1.FrameDescriptor) string {
	for _, p := range frame.GetProvenanceAttestations() {
		if len(p) > 7 && p[:7] == "tenant=" {
			return p[7:]
		}
	}
	return ""
}
