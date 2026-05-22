// Emitter operator: serializes events.v1 entries to protobuf and publishes
// them on the bus. This is the final hop in the per-frame chain and the
// point at which the wall-clock latency from capture to event is fixed.
package operators

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/pkg/dataplane"
	"google.golang.org/protobuf/proto"
)

// EmitConfig wires a Publisher and the canonical events.v1 subject.
type EmitConfig struct {
	Publisher bus.Publisher
	Subject   string // defaults to events.v1
	Logger    *slog.Logger
}

type Emit struct {
	cfg EmitConfig
}

func NewEmit(cfg EmitConfig) *Emit {
	if cfg.Subject == "" {
		cfg.Subject = "events.v1"
	}
	return &Emit{cfg: cfg}
}

func (e *Emit) Name() string { return "emit" }

func (e *Emit) Run(ctx context.Context, in <-chan *dataplane.FrameEnvelope, out chan<- *dataplane.FrameEnvelope) error {
	if e.cfg.Publisher == nil {
		return fmt.Errorf("emit: publisher is required")
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case env, ok := <-in:
			if !ok {
				return dataplane.ErrUpstreamClosed
			}
			for _, ev := range env.RuleEvents {
				data, err := proto.Marshal(ev)
				if err != nil {
					if e.cfg.Logger != nil {
						e.cfg.Logger.Warn("event marshal failed", slog.String("err", err.Error()))
					}
					continue
				}
				if e.cfg.Logger != nil {
					e.cfg.Logger.Info("emit",
						slog.String("rule", ev.GetRuleId()),
						slog.String("stream", ev.GetStreamId()),
						slog.Int64("frame", ev.GetFrameSeq()),
						slog.String("subject", e.cfg.Subject),
					)
				}
				headers := map[string]string{
					"tenant":   ev.GetTenantId(),
					"stream":   ev.GetStreamId(),
					"pipeline": ev.GetPipelineId(),
				}
				if tp := ev.GetTraceparent(); tp != "" {
					headers["traceparent"] = tp
				}
				if err := e.cfg.Publisher.Publish(ctx, bus.Message{
					Subject: e.cfg.Subject,
					Data:    data,
					Headers: headers,
				}); err != nil {
					if e.cfg.Logger != nil {
						e.cfg.Logger.Warn("event publish failed", slog.String("err", err.Error()))
					}
				}
			}
			env.EmittedAt = nowFn()
			select {
			case <-ctx.Done():
				return nil
			case out <- env:
			}
		}
	}
}
