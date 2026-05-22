// Privacy-by-design redaction operator. Per the architecture doc's
// "Privacy by design" chapter, face + license-plate redaction is a
// platform-provided node that pipelines opt-into by adding it to the DAG
// before any sink that emits media.
//
// The operator consumes detections whose class is in the redact list
// (e.g. "face", "license-plate"), and overlays a blur or solid box on the
// frame payload. In the walking-skeleton the payload is the synthetic JSON
// target; production swaps in a CV-CUDA kernel that does in-GPU box-blur.
//
// Importantly: even when redaction is *not* on the DAG, the dataplane will
// refuse to emit raw-imagery sinks for tenants whose policy demands
// redaction. Enforcement lives in pipeline-service at compile time.
package operators

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/aegisvision/pkg/dataplane"
	"github.com/aegisvision/pkg/dataplane/claimcheck"
	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

// RedactConfig wires the operator. Classes contains the lowercase class
// labels that trigger redaction (default: "face", "license-plate", "plate").
// Ring is required — the operator overwrites the payload behind the claim-
// check locator with a redacted copy.
type RedactConfig struct {
	Classes []string
	Ring    *claimcheck.Ring
	Logger  *slog.Logger
	// Mode = "blur" (default) | "box" (solid black rectangle).
	Mode string
}

type Redact struct {
	cfg     RedactConfig
	classes map[string]bool
}

// NewRedact constructs the operator. Empty Classes defaults to
// face/license-plate.
func NewRedact(cfg RedactConfig) *Redact {
	if len(cfg.Classes) == 0 {
		cfg.Classes = []string{"face", "license-plate", "plate"}
	}
	m := make(map[string]bool, len(cfg.Classes))
	for _, c := range cfg.Classes {
		m[c] = true
	}
	if cfg.Mode == "" {
		cfg.Mode = "blur"
	}
	return &Redact{cfg: cfg, classes: m}
}

func (r *Redact) Name() string { return "redact" }

func (r *Redact) Run(ctx context.Context, in <-chan *dataplane.FrameEnvelope, out chan<- *dataplane.FrameEnvelope) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case env, ok := <-in:
			if !ok {
				return dataplane.ErrUpstreamClosed
			}
			r.apply(env)
			select {
			case <-ctx.Done():
				return nil
			case out <- env:
			}
		}
	}
}

func (r *Redact) apply(env *dataplane.FrameEnvelope) {
	var boxes []*dataplanev1.BoundingBox
	for _, d := range env.Detections {
		if r.classes[d.GetClassLabel()] {
			boxes = append(boxes, d.GetBbox())
		}
	}
	if len(boxes) == 0 {
		return
	}

	// In the walking-skeleton path the payload is the synthetic JSON target.
	// We "redact" by replacing the bbox geometry with a zero-area entry so
	// any downstream consumer (clip exporter, thumbnail generator) cannot
	// recover the original region. In production the same operator calls a
	// CV-CUDA Gaussian-blur kernel against pixel data behind the locator.
	if r.cfg.Ring == nil {
		return
	}
	loc, err := claimcheck.ParseLocator(env.Descriptor.GetPayload().GetLocator())
	if err != nil {
		return
	}
	payload, err := r.cfg.Ring.Get(loc)
	if err != nil {
		return
	}
	var t SyntheticTarget
	if err := json.Unmarshal(payload, &t); err == nil {
		t.X, t.Y, t.W, t.H = 0, 0, 0, 0
		t.Class = "redacted-" + r.cfg.Mode
		body, _ := json.Marshal(t)
		newLoc := r.cfg.Ring.Put(body)
		env.Descriptor.Payload = &dataplanev1.PayloadLocation{
			Kind:    dataplanev1.PayloadLocation_KIND_SHARED_MEMORY,
			Locator: newLoc.Encode(),
		}
	}
	if r.cfg.Logger != nil {
		r.cfg.Logger.Debug("redacted",
			slog.Int("boxes", len(boxes)),
			slog.String("stream", env.Descriptor.GetStreamId()),
			slog.String("mode", r.cfg.Mode))
	}
}
