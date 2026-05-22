// Package dataplane is the streaming runtime that hosts the per-frame
// operator DAG.
//
// Per ADR-0001, this package runs the data plane: bounded channels between
// operators, backpressure as a first-class signal, and no per-step durable
// persistence. The control plane (pipeline-service + stream-manager) hands
// us a compiled graph; we run it at line rate.
//
// Operator contract:
//
//  1. Operators receive frames over a Go channel.
//  2. Operators emit frames over a Go channel.
//  3. Channels are bounded. Full channels block the producer — that's the
//     backpressure mechanism. Operators MUST NOT silently drop on a full
//     downstream channel; if they need to drop, they drop with metrics.
//  4. Operators do not retain references to a frame across iterations
//     unless they explicitly take a copy.
//  5. Operators stop when their input channel closes; they close their
//     output channel before returning so the next operator can drain.
package dataplane

import (
	"context"
	"errors"
	"time"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

// FrameEnvelope is the in-process envelope around a FrameDescriptor that
// carries operator-local mutable state (detections, tracks) accreted along
// the chain. The wire payload between processes is the embedded Descriptor
// only; everything else is local to this runner.
type FrameEnvelope struct {
	Descriptor *dataplanev1.FrameDescriptor

	// Detections accumulated by upstream model operators.
	Detections []*dataplanev1.Detection

	// Tracks assigned by the tracker operator.
	Tracks []TrackHit

	// RuleEvents are events accumulated by the rule operator for this frame;
	// the emitter ships them on the events.v1 subject.
	RuleEvents []*dataplanev1.Event

	// EmittedAt is set by the emitter once it ships an event; surfaces as
	// the emit latency metric.
	EmittedAt time.Time
}

// TrackHit is a (detection -> track id) assignment with stable ID across
// frames. Tracker operators write this; rule operators read it.
type TrackHit struct {
	DetectionIndex int
	TrackID        string
}

// Operator is one node in the DAG. Inputs[i] is read until close; Outputs[i]
// is written and then closed on exit. For the linear chain in the walking
// skeleton Operators have exactly one input and one output channel; the
// runtime is structured to allow fan-out / fan-in later without changing
// the interface.
type Operator interface {
	// Name is used for metrics labels and logs; must be stable across runs.
	Name() string
	// Run executes the operator. It must return when ctx is canceled or
	// when its input channel closes, whichever comes first.
	Run(ctx context.Context, in <-chan *FrameEnvelope, out chan<- *FrameEnvelope) error
}

// ErrUpstreamClosed is the sentinel an operator can return to indicate it
// stopped because its upstream closed — the normal end-of-stream path.
var ErrUpstreamClosed = errors.New("dataplane: upstream closed")
