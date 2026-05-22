package operators_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/aegisvision/pkg/dataplane"
	"github.com/aegisvision/pkg/dataplane/claimcheck"
	"github.com/aegisvision/pkg/dataplane/operators"
	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

func TestRedact_RewritesPayloadForConfiguredClasses(t *testing.T) {
	ring, _ := claimcheck.NewRing("s", 8)
	loc := ring.Put([]byte(`{"X":0.5,"Y":0.5,"W":0.1,"H":0.1,"Class":"face","Score":0.9}`))
	op := operators.NewRedact(operators.RedactConfig{Ring: ring})

	captured := []*dataplane.FrameEnvelope{}
	var mu sync.Mutex
	sink := captureSink(func(env *dataplane.FrameEnvelope) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, env)
	})

	p := &dataplane.Pipeline{ID: "p", BufferSize: 8, Operators: []dataplane.Operator{op, sink}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	src, wait := p.Run(ctx)
	src <- &dataplane.FrameEnvelope{
		Descriptor: &dataplanev1.FrameDescriptor{
			StreamId: "s", FrameSeq: 1,
			Payload: &dataplanev1.PayloadLocation{Locator: loc.Encode()},
		},
		Detections: []*dataplanev1.Detection{
			{ClassLabel: "face", Bbox: &dataplanev1.BoundingBox{X: 0.5, Y: 0.5, W: 0.1, H: 0.1}},
		},
	}
	close(src)
	_ = wait()

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 {
		t.Fatalf("want 1 envelope, got %d", len(captured))
	}
	newLoc, err := claimcheck.ParseLocator(captured[0].Descriptor.GetPayload().GetLocator())
	if err != nil {
		t.Fatal(err)
	}
	body, err := ring.Get(newLoc)
	if err != nil {
		t.Fatal(err)
	}
	var t2 operators.SyntheticTarget
	_ = json.Unmarshal(body, &t2)
	if t2.W != 0 || t2.H != 0 {
		t.Fatalf("redaction did not zero the box: %+v", t2)
	}
	if t2.Class != "redacted-blur" {
		t.Fatalf("redaction marker missing: %q", t2.Class)
	}
}

func TestRedact_IgnoresUnconfiguredClasses(t *testing.T) {
	ring, _ := claimcheck.NewRing("s", 8)
	loc := ring.Put([]byte(`{"X":0.5,"Y":0.5,"W":0.2,"H":0.2,"Class":"vehicle","Score":0.9}`))
	op := operators.NewRedact(operators.RedactConfig{Ring: ring, Classes: []string{"face"}})

	captured := []*dataplane.FrameEnvelope{}
	var mu sync.Mutex
	sink := captureSink(func(env *dataplane.FrameEnvelope) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, env)
	})

	p := &dataplane.Pipeline{ID: "p", BufferSize: 8, Operators: []dataplane.Operator{op, sink}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	src, wait := p.Run(ctx)
	src <- &dataplane.FrameEnvelope{
		Descriptor: &dataplanev1.FrameDescriptor{
			StreamId: "s", FrameSeq: 1,
			Payload: &dataplanev1.PayloadLocation{Locator: loc.Encode()},
		},
		Detections: []*dataplanev1.Detection{
			{ClassLabel: "vehicle", Bbox: &dataplanev1.BoundingBox{X: 0.5, Y: 0.5, W: 0.2, H: 0.2}},
		},
	}
	close(src)
	_ = wait()

	mu.Lock()
	defer mu.Unlock()
	// Vehicle is not in the redact set; payload locator must be unchanged.
	if captured[0].Descriptor.GetPayload().GetLocator() != loc.Encode() {
		t.Fatalf("redaction ran for non-configured class")
	}
}
