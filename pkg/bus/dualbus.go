// DualBus routes each subject to one of the underlying buses by name.
//
// The convention from the architecture doc §17:
//
//   Kafka (durable):  events.v1, audit.v1, detections.v1, tracks.v1, …
//   NATS  (RPC/low-latency): frame.descriptor.*, operator.control.*, …
//
// Callers don't care which transport carries the bytes; the DualBus picks
// the right one from the configured routing table and forwards.
package bus

import (
	"context"
	"errors"
	"strings"
)

// Route maps a subject prefix to a target bus. Routes are matched in
// declaration order; the first prefix match wins.
type Route struct {
	Prefix string
	Bus    Publisher
}

// DualBus implements Publisher by routing on Subject prefix.
type DualBus struct {
	Routes  []Route
	Default Publisher // fallback when no prefix matches
}

// Publish dispatches according to the routing table.
func (d *DualBus) Publish(ctx context.Context, msg Message) error {
	for _, r := range d.Routes {
		if strings.HasPrefix(msg.Subject, r.Prefix) {
			return r.Bus.Publish(ctx, msg)
		}
	}
	if d.Default == nil {
		return errors.New("bus: no route for subject " + msg.Subject)
	}
	return d.Default.Publish(ctx, msg)
}

// Close closes every underlying publisher.
func (d *DualBus) Close() error {
	var first error
	for _, r := range d.Routes {
		if err := r.Bus.Close(); err != nil && first == nil {
			first = err
		}
	}
	if d.Default != nil {
		if err := d.Default.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
