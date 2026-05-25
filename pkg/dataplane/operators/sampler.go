// The sampler clamps the per-stream FPS to a budget. It is the simplest
// shape of an "adaptive sampler"; a future revision swaps in a feedback
// loop that raises and lowers the sample rate based on inference
// pressure.
package operators

import (
	"context"

	"github.com/aegisvision/pkg/dataplane"
)

// SamplerConfig caps frames at a target FPS. The math: emit one frame every
// ceil(sourceFPS / targetFPS) frames.
type SamplerConfig struct {
	SourceFPS int
	TargetFPS int
}

type Sampler struct {
	cfg SamplerConfig
}

func NewSampler(cfg SamplerConfig) *Sampler {
	if cfg.TargetFPS <= 0 || cfg.TargetFPS > cfg.SourceFPS {
		cfg.TargetFPS = cfg.SourceFPS
	}
	return &Sampler{cfg: cfg}
}

func (s *Sampler) Name() string { return "sampler" }

func (s *Sampler) Run(ctx context.Context, in <-chan *dataplane.FrameEnvelope, out chan<- *dataplane.FrameEnvelope) error {
	step := 1
	if s.cfg.TargetFPS > 0 {
		step = s.cfg.SourceFPS / s.cfg.TargetFPS
		if step < 1 {
			step = 1
		}
	}
	var i int64
	for {
		select {
		case <-ctx.Done():
			return nil
		case env, ok := <-in:
			if !ok {
				return dataplane.ErrUpstreamClosed
			}
			if i%int64(step) == 0 {
				select {
				case <-ctx.Done():
					return nil
				case out <- env:
				}
			}
			i++
		}
	}
}
