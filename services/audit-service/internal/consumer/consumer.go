// Package consumer subscribes to the audit.v1 bus subject and appends
// every record to the append-only audit store.
//
// Two delivery modes:
//
//  1. Standard subscription (queue group "audit") — at-least-once;
//     records that fail to insert get redelivered. Duplicates are
//     caught by the audit_records primary-key constraint on (id).
//  2. Wildcard ingress for legacy producers that publish to
//     "audit.<category>.v1" prefixes. Folded into the same store.
//
// Per the architecture doc + ADR-0014, audit is the evidence record
// for compliance: we MUST NOT drop messages silently. Insert failures
// are surfaced via the redeliver counter and the bus's at-least-once
// contract — the consumer returns an error from its handler so the
// bus replays.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/services/audit-service/internal/store"
)

// Subject is the canonical audit topic. Other prefixes are merged here
// by listening to "audit.>".
const Subject = "audit.v1"

// Wildcard is the legacy compatibility prefix.
const Wildcard = "audit.>"

// Inserter abstracts the store side for tests.
type Inserter interface {
	AppendJSON(ctx context.Context, body []byte) error
}

// Consumer wires the subscription.
type Consumer struct {
	Logger  *slog.Logger
	Sub     bus.Subscriber
	Store   Inserter
	// QueueGroup is the durable consumer group; every audit-service
	// replica must share the same group.
	QueueGroup string

	subs []bus.Subscription
}

// New constructs a Consumer with the default queue group ("audit").
func New(logger *slog.Logger, sub bus.Subscriber, store Inserter) *Consumer {
	return &Consumer{Logger: logger, Sub: sub, Store: store, QueueGroup: "audit"}
}

// Start subscribes to Subject + Wildcard. Returns the first error.
func (c *Consumer) Start(ctx context.Context) error {
	if c.Sub == nil || c.Store == nil {
		return errors.New("audit consumer: missing sub or store")
	}
	handler := func(ctx context.Context, msg bus.Message) error {
		// We trust the producer's JSON shape — the middleware that
		// generates these records is the canonical writer.
		if err := c.Store.AppendJSON(ctx, msg.Data); err != nil {
			// Duplicates (PK conflicts on id) are NOT errors — the
			// consumer is at-least-once + the table has a unique id
			// per record.
			if isDuplicate(err) {
				return nil
			}
			if c.Logger != nil {
				c.Logger.WarnContext(ctx, "audit append failed", slog.String("err", err.Error()))
			}
			return err
		}
		return nil
	}
	s1, err := c.Sub.Subscribe(ctx, Subject, c.QueueGroup, handler)
	if err != nil {
		return err
	}
	c.subs = append(c.subs, s1)
	s2, err := c.Sub.Subscribe(ctx, Wildcard, c.QueueGroup, handler)
	if err != nil {
		return err
	}
	c.subs = append(c.subs, s2)
	return nil
}

// Close drains the subscriptions. The bus's at-least-once contract
// ensures any in-flight ack will be re-attempted by the next consumer.
func (c *Consumer) Close() error {
	for _, s := range c.subs {
		_ = s.Drain()
	}
	return nil
}

// isDuplicate inspects an error for the PK-conflict signature across
// the supported drivers. We accept both the SQLite ("UNIQUE
// constraint failed") and the pgx ("23505" SQLSTATE) shapes.
func isDuplicate(err error) bool {
	s := err.Error()
	return contains(s, "UNIQUE constraint failed") ||
		contains(s, "duplicate key value") ||
		contains(s, "SQLSTATE 23505")
}

func contains(s, sub string) bool {
	return len(sub) <= len(s) && (s == sub || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Compile-time touch so the time package isn't unused in some build configurations.
var _ = time.Now
var _ = json.Unmarshal
var _ = fmt.Sprint
var _ = store.Record{}
