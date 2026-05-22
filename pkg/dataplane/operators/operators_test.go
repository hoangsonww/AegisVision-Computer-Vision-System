package operators_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/pkg/dataplane"
	"github.com/aegisvision/pkg/dataplane/claimcheck"
	"github.com/aegisvision/pkg/dataplane/operators"
	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
	"google.golang.org/protobuf/proto"
)

// Full chain test: ingest -> sampler -> detect -> tracker -> rule -> emit
// runs end-to-end with the in-memory bus and a synthetic detector. The test
// asserts that a rule fires for a target that dwells inside a zone.
func TestFullChain_DwellRuleEmitsEvent(t *testing.T) {
	reg := claimcheck.NewRegistry()
	ring, err := reg.Ensure("stream-1", 64)
	if err != nil {
		t.Fatal(err)
	}

	b := bus.NewInMemory()
	defer b.Close()

	received := make(chan *dataplanev1.Event, 16)
	_, err = b.Subscribe(context.Background(), "events.v1", "", func(_ context.Context, m bus.Message) error {
		var ev dataplanev1.Event
		if err := proto.Unmarshal(m.Data, &ev); err == nil {
			received <- &ev
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	ops := []dataplane.Operator{
		operators.NewIngest(operators.IngestConfig{
			StreamID:   "stream-1",
			PipelineID: "p-test",
			Width:      640, Height: 360,
			FPS:   60,
			Total: 180, // 3 seconds at 60fps
			Ring:  ring,
		}),
		operators.NewSampler(operators.SamplerConfig{SourceFPS: 60, TargetFPS: 30}),
		operators.NewDetect(operators.DetectConfig{
			Detector: operators.NewSyntheticDetector(),
			Ring:     ring,
		}),
		operators.NewTracker(operators.TrackerConfig{IOUThreshold: 0.05}),
		operators.NewRule(operators.RuleConfig{
			Rules: []*operators.SpatialDwellRule{{
				RuleID:   "dwell-1",
				TenantID: "t-test",
				Zone:     operators.Zone{ID: "right-half", X: 0.5, Y: 0.0, W: 0.5, H: 1.0},
				Dwell:    400 * time.Millisecond,
				Severity: commonv1.Severity_SEVERITY_HIGH,
			}},
		}),
		operators.NewEmit(operators.EmitConfig{Publisher: b}),
	}

	p := &dataplane.Pipeline{ID: "p-test", Operators: ops, BufferSize: 32}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	source, wait := p.Run(ctx)
	close(source) // synthetic ingest ignores its input; closing kicks the chain to finish on Total

	if err := wait(); err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	select {
	case ev := <-received:
		if ev.GetTenantId() != "t-test" {
			t.Fatalf("tenant: %q", ev.GetTenantId())
		}
		if ev.GetRuleId() != "dwell-1" {
			t.Fatalf("rule: %q", ev.GetRuleId())
		}
		if ev.GetSeverity() != commonv1.Severity_SEVERITY_HIGH {
			t.Fatalf("severity: %v", ev.GetSeverity())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no rule event received within 3s — chain is broken")
	}
}

// SyntheticDetector + Tracker — track IDs are stable across consecutive
// detections of the same target.
func TestTracker_StableIDAcrossFrames(t *testing.T) {
	ring, _ := claimcheck.NewRing("s", 16)

	// Capture-only sink so we can read every envelope after the chain runs.
	captured := []*dataplane.FrameEnvelope{}
	var capMu sync.Mutex
	sink := captureSink(func(env *dataplane.FrameEnvelope) {
		capMu.Lock()
		captured = append(captured, env)
		capMu.Unlock()
	})

	ops := []dataplane.Operator{
		operators.NewDetect(operators.DetectConfig{Detector: operators.NewSyntheticDetector(), Ring: ring}),
		operators.NewTracker(operators.TrackerConfig{IOUThreshold: 0.1}),
		sink,
	}
	p := &dataplane.Pipeline{ID: "t", Operators: ops, BufferSize: 16}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	source, wait := p.Run(ctx)
	for i := 0; i < 5; i++ {
		loc := ring.Put([]byte(`{"X":0.5,"Y":0.5,"W":0.1,"H":0.1,"Class":"person","Score":0.9}`))
		source <- &dataplane.FrameEnvelope{
			Descriptor: &dataplanev1.FrameDescriptor{
				StreamId: "s", FrameSeq: int64(i),
				Payload: &dataplanev1.PayloadLocation{Locator: loc.Encode()},
			},
		}
	}
	close(source)
	if err := wait(); err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	seenIDs := map[string]bool{}
	capMu.Lock()
	for _, env := range captured {
		for _, h := range env.Tracks {
			seenIDs[h.TrackID] = true
		}
	}
	capMu.Unlock()
	if len(seenIDs) != 1 {
		t.Fatalf("want 1 stable track id, got %d: %v", len(seenIDs), seenIDs)
	}
}

type capOp struct {
	name string
	fn   func(*dataplane.FrameEnvelope)
}

func (c *capOp) Name() string { return c.name }
func (c *capOp) Run(ctx context.Context, in <-chan *dataplane.FrameEnvelope, out chan<- *dataplane.FrameEnvelope) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case env, ok := <-in:
			if !ok {
				return dataplane.ErrUpstreamClosed
			}
			c.fn(env)
			select {
			case <-ctx.Done():
				return nil
			case out <- env:
			}
		}
	}
}
func captureSink(fn func(*dataplane.FrameEnvelope)) dataplane.Operator {
	return &capOp{name: "sink", fn: fn}
}
