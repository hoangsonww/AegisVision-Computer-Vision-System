// Spatial dwell rule operator. Mirrors the YAML example from the
// architecture doc §27.2: emit a HIGH-severity event when a tracked object
// dwells inside a configured zone for at least N seconds.
//
// The dedicated rule-engine service generalizes this to a composable
// expression language; the operator here demonstrates the
// rule -> event projection that the engine must also satisfy.
package operators

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/aegisvision/pkg/dataplane"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Zone is a rectangle in normalized [0,1] coordinates.
type Zone struct {
	ID         string
	X, Y, W, H float32
}

// SpatialDwellRule fires when a track has been continuously inside Zone for
// at least Dwell duration.
type SpatialDwellRule struct {
	RuleID   string
	TenantID string
	Zone     Zone
	Dwell    time.Duration
	Severity commonv1.Severity
}

// RuleConfig wires a rule into the operator. Future versions accept many
// rules.
type RuleConfig struct {
	Rules  []*SpatialDwellRule
	Logger *slog.Logger
}

type Rule struct {
	cfg RuleConfig

	mu      sync.Mutex
	insides map[string]ruleState // key = trackID + ":" + ruleID
}

type ruleState struct {
	enteredAt time.Time
	fired     bool
}

func NewRule(cfg RuleConfig) *Rule {
	return &Rule{cfg: cfg, insides: map[string]ruleState{}}
}

func (r *Rule) Name() string { return "rule" }

func (r *Rule) Run(ctx context.Context, in <-chan *dataplane.FrameEnvelope, out chan<- *dataplane.FrameEnvelope) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case env, ok := <-in:
			if !ok {
				return dataplane.ErrUpstreamClosed
			}
			r.evaluate(env)
			select {
			case <-ctx.Done():
				return nil
			case out <- env:
			}
		}
	}
}

func (r *Rule) evaluate(env *dataplane.FrameEnvelope) {
	now := env.Descriptor.GetCaptureTime().AsTime()
	if now.IsZero() {
		now = time.Now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, rule := range r.cfg.Rules {
		for _, det := range env.Detections {
			if det.GetTrackId() == "" {
				continue
			}
			key := det.GetTrackId() + ":" + rule.RuleID
			if !contains(rule.Zone, det.GetBbox()) {
				delete(r.insides, key)
				continue
			}
			s, ok := r.insides[key]
			if !ok {
				r.insides[key] = ruleState{enteredAt: now}
				if r.cfg.Logger != nil {
					r.cfg.Logger.Info("rule entered zone",
						slog.String("rule", rule.RuleID),
						slog.String("track", det.GetTrackId()),
						slog.Int64("frame", env.Descriptor.GetFrameSeq()))
				}
				continue
			}
			if !s.fired && now.Sub(s.enteredAt) >= rule.Dwell {
				ev := buildEvent(rule, env.Descriptor, det)
				env.Detections[0] = det
				env.RuleEvents = append(env.RuleEvents, ev)
				s.fired = true
				r.insides[key] = s
				if r.cfg.Logger != nil {
					r.cfg.Logger.Info("rule fired",
						slog.String("rule", rule.RuleID),
						slog.String("track", det.GetTrackId()),
						slog.Duration("dwell", now.Sub(s.enteredAt)))
				}
			}
		}
	}
}

// contains is true when the bounding-box center is inside the zone.
func contains(z Zone, b *dataplanev1.BoundingBox) bool {
	if b == nil {
		return false
	}
	cx := b.GetX() + b.GetW()/2
	cy := b.GetY() + b.GetH()/2
	return cx >= z.X && cx <= z.X+z.W && cy >= z.Y && cy <= z.Y+z.H
}

func buildEvent(rule *SpatialDwellRule, frame *dataplanev1.FrameDescriptor, det *dataplanev1.Detection) *dataplanev1.Event {
	return &dataplanev1.Event{
		Id:          "evt-" + newRandHex(8),
		TenantId:    rule.TenantID,
		PipelineId:  frame.GetPipelineId(),
		StreamId:    frame.GetStreamId(),
		Kind:        dataplanev1.EventKind_EVENT_KIND_RULE_FIRED,
		Severity:    rule.Severity,
		FrameSeq:    frame.GetFrameSeq(),
		CaptureTime: frame.GetCaptureTime(),
		EmittedAt:   timestamppb.Now(),
		Summary:     "dwell rule fired",
		RuleId:      rule.RuleID,
		ZoneId:      rule.Zone.ID,
		TrackId:     det.GetTrackId(),
		ClassLabel:  det.GetClassLabel(),
		Score:       det.GetScore(),
		BboxX:       det.GetBbox().GetX(),
		BboxY:       det.GetBbox().GetY(),
		BboxW:       det.GetBbox().GetW(),
		BboxH:       det.GetBbox().GetH(),
		Traceparent: frame.GetTraceparent(),
	}
}

func newRandHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
