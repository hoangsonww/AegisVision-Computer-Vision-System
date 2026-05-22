// Synthetic ingest operator for the walking skeleton.
//
// Real Phase 2 ingest will run NVIDIA DeepStream and feed NVDEC-decoded GPU
// memory into the first CUDA-IPC operator. For the walking skeleton we
// generate a deterministic moving target whose pixel data is a tiny JSON
// blob written into the claim-check ring — the descriptor still travels
// the same path, and downstream operators consume it without knowing the
// difference.
package operators

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aegisvision/pkg/dataplane"
	"github.com/aegisvision/pkg/dataplane/claimcheck"
	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SyntheticTarget is the JSON-encoded payload behind each synthetic frame.
// In a real frame this would be NV12 / RGBA pixels; the schema is identical
// from the operator's perspective — opaque bytes behind a locator.
type SyntheticTarget struct {
	X, Y, W, H float32
	Class      string
	Score      float32
}

// IngestConfig configures the synthetic source.
type IngestConfig struct {
	StreamID   string
	PipelineID string
	Width      int32
	Height     int32
	// FPS is the wall-clock generation rate; the operator sleeps between
	// frames to honor it.
	FPS int
	// Total bounds how many frames to produce; -1 for unbounded.
	Total int

	// Ring is required — the operator writes its payload there and emits
	// the locator on the descriptor.
	Ring *claimcheck.Ring

	Logger *slog.Logger
}

// Ingest is the operator. It ignores its input channel and produces frames
// based purely on its config.
type Ingest struct {
	cfg IngestConfig
}

func NewIngest(cfg IngestConfig) *Ingest { return &Ingest{cfg: cfg} }

func (i *Ingest) Name() string { return "ingest" }

// Run produces frames at the configured FPS, walking a synthetic target
// from left to right across the frame. The target dwells in the right-hand
// half of the frame after frame 60 so a downstream zone rule can fire.
func (i *Ingest) Run(ctx context.Context, _ <-chan *dataplane.FrameEnvelope, out chan<- *dataplane.FrameEnvelope) error {
	if i.cfg.Ring == nil {
		return fmt.Errorf("ingest: ring is required")
	}
	if i.cfg.FPS <= 0 {
		i.cfg.FPS = 30
	}
	tick := time.NewTicker(time.Second / time.Duration(i.cfg.FPS))
	defer tick.Stop()

	for seq := int64(0); ; seq++ {
		if i.cfg.Total > 0 && seq >= int64(i.cfg.Total) {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case now := <-tick.C:
			target := walkPattern(seq, i.cfg.Width, i.cfg.Height)
			if i.cfg.Logger != nil && seq%30 == 0 {
				i.cfg.Logger.Debug("ingest tick", slog.Int64("seq", seq), slog.Float64("x", float64(target.X)))
			}
			payload, _ := json.Marshal(target)
			loc := i.cfg.Ring.Put(payload)

			desc := &dataplanev1.FrameDescriptor{
				StreamId:    i.cfg.StreamID,
				PipelineId:  i.cfg.PipelineID,
				FrameSeq:    seq,
				CaptureTime: timestamppb.New(now),
				Width:       i.cfg.Width,
				Height:      i.cfg.Height,
				PixelFormat: "synthetic.json",
				ColorSpace:  "n/a",
				Payload: &dataplanev1.PayloadLocation{
					Kind:    dataplanev1.PayloadLocation_KIND_SHARED_MEMORY,
					Locator: loc.Encode(),
				},
			}
			env := &dataplane.FrameEnvelope{Descriptor: desc}
			select {
			case <-ctx.Done():
				return nil
			case out <- env:
			}
		}
	}
}

// walkPattern produces a deterministic oscillating trajectory. The target
// crosses into the right-half zone, dwells, then crosses back out. Each
// cycle re-arms the dwell rule, so a long-running pipeline emits one event
// per cycle — ideal for SSE/console verification.
//
// At 30 fps the cycle is 10s; with the 2s dwell rule, one event per 10s.
func walkPattern(seq int64, w, h int32) SyntheticTarget {
	_ = w
	_ = h
	const cycle = 300
	phase := seq % cycle
	var x float32
	switch {
	case phase < 120:
		x = 0.05 + float32(phase)*0.7/120
	case phase < 150:
		x = 0.75
	case phase < 270:
		x = 0.75 - float32(phase-150)*0.7/120
	default:
		x = 0.05
	}
	return SyntheticTarget{
		X:     x,
		Y:     0.45,
		W:     0.12,
		H:     0.30,
		Class: "person",
		Score: 0.92,
	}
}
