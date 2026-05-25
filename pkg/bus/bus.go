// Package bus is the platform's abstraction over the event backbone.
//
// Per the architecture doc §17, AegisVision runs two buses, used for two jobs:
//
//   - Kafka:           durable, ordered-per-partition, at-least-once delivery
//                      log. Carries the long-tail topics (events.v1, audit.v1,
//                      detections.v1, model.lifecycle.v1, ...).
//   - NATS JetStream:  low-latency RPC + agent + operator control bus. Carries
//                      frame.descriptor.<stream_id>, operator.control.<pipeline>,
//                      cluster.heartbeat.
//
// The walking-skeleton lands on **NATS only** — events.v1 included — because
// NATS JetStream gives us at-least-once + replay without standing up Kafka.
// When Kafka is added (production-shape deployments), only the wiring at
// startup changes; the Publisher / Subscriber interfaces below are
// deliberately bus-agnostic.
//
// Subject naming (subject == "topic" in NATS lingo):
//
//	events.v1                       — all platform events (filterable by tenant/stream)
//	detections.v1                   — single-frame detections firehose
//	frame.descriptor.<stream_id>    — claim-check envelopes for one stream
//	operator.control.<pipeline_id>  — operator lifecycle signals from control plane
//	pipeline.lifecycle.v1           — pipeline created/deployed/paused/...
package bus

import (
	"context"
	"errors"
)

// Message is a published payload + structured metadata. Headers map to NATS
// message headers and Kafka record headers identically.
type Message struct {
	Subject string
	Data    []byte
	Headers map[string]string
}

// Publisher is the producer-side abstraction.
type Publisher interface {
	Publish(ctx context.Context, msg Message) error
	Close() error
}

// Subscriber is the consumer-side abstraction. Each Subscribe call yields a
// Subscription that delivers messages onto the supplied handler. The handler
// must return nil for ack; returning a non-nil error triggers a redelivery
// per the bus's at-least-once contract.
type Subscriber interface {
	Subscribe(ctx context.Context, subject, queueGroup string, handler Handler) (Subscription, error)
	Close() error
}

// Handler processes one message. Returning a non-nil error nacks the message;
// returning ErrSkip drops it without redelivery (e.g. for unparseable
// payloads).
type Handler func(ctx context.Context, msg Message) error

// ErrSkip drops a message without redelivery.
var ErrSkip = errors.New("bus: skip without redelivery")

// Subscription is the handle returned from Subscribe. Drain() flushes
// in-flight messages and unsubscribes; Unsubscribe() drops immediately.
type Subscription interface {
	Drain() error
	Unsubscribe() error
}
