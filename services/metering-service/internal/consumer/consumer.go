// Package consumer subscribes to usage-event subjects on the platform
// bus and increments the corresponding daily counters in the store.
//
// Why this lives here rather than in main.go: the consumer is the bulk
// of the service's runtime work, and it's the natural unit to test
// without spinning a full server. main.go wires it up; this package owns
// the message handling + per-tenant batching.
//
// Subjects we listen to (all carry tenant_id in headers):
//
//   - events.v1                — billable platform events (count: events)
//   - detections.v1            — billable detections (count: detections)
//   - inference.completed.v1   — billable inferences (count: inferences)
//   - llm.completed.v1         — LLM token usage (count: prompt_tokens + completion_tokens)
//   - learning.labeled.v1      — billable annotations (count: annotations)
//
// We *batch* increments in-memory and flush every Flush interval (or
// when the buffer exceeds MaxBuffered). Without batching, every
// detection on the firehose becomes its own SQL UPDATE — which the
// architecture doc explicitly warns against (Postgres alone cannot hold
// 30 fps × 10k streams; see ADR-0002).
package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/aegisvision/pkg/bus"
)

// Incrementer abstracts the store side so we can test the consumer with
// an in-memory recorder.
type Incrementer interface {
	Increment(ctx context.Context, tenant, metric string, n int64) error
}

// Subscriptions is the default set of subjects the metering consumer
// follows. Production overrides this only for narrow experiments — the
// list is the canonical billable-event catalog.
var Subscriptions = []Subscription{
	{Subject: "events.v1", Metric: "events"},
	{Subject: "detections.v1", Metric: "detections"},
	{Subject: "inference.completed.v1", Metric: "inferences"},
	{Subject: "learning.labeled.v1", Metric: "annotations"},
	// LLM usage is special — count must come from the message body's
	// `total_tokens` field, not "+1 per message".
	{Subject: "llm.completed.v1", Metric: "llm_tokens", TokenField: "total_tokens"},
}

// Subscription describes one (subject, metric) binding.
type Subscription struct {
	Subject string
	// Metric is the counter name written to the store. Convention:
	// short lowercase nouns ("events", "detections", "inferences",
	// "annotations", "llm_tokens").
	Metric string
	// TokenField, if set, instructs the consumer to use that JSON field
	// from the message body as the increment count rather than 1.
	TokenField string
	// Optional queue group (defaults to "metering"). All metering
	// replicas should share the queue group so a message is processed
	// once per cluster.
	QueueGroup string
}

// Consumer batches inbound increments and flushes them periodically.
type Consumer struct {
	Logger *slog.Logger
	Sub    bus.Subscriber
	Inc    Incrementer
	// Flush is the periodic flush interval. Defaults to 1s.
	Flush time.Duration
	// MaxBuffered is the per-(tenant, metric) cap before a synchronous
	// flush is forced. Defaults to 1024.
	MaxBuffered int
	// Subscriptions to follow. Defaults to package-level Subscriptions.
	Subscriptions []Subscription

	mu  sync.Mutex
	buf map[bufKey]int64

	subs []bus.Subscription
}

type bufKey struct{ tenant, metric string }

// New constructs a Consumer with sensible defaults.
func New(logger *slog.Logger, sub bus.Subscriber, inc Incrementer) *Consumer {
	return &Consumer{
		Logger:        logger,
		Sub:           sub,
		Inc:           inc,
		Flush:         1 * time.Second,
		MaxBuffered:   1024,
		Subscriptions: Subscriptions,
		buf:           map[bufKey]int64{},
	}
}

// Start subscribes to every Subscription and launches the periodic
// flusher. Returns the first subscription error if any. The flusher
// runs until ctx is canceled.
func (c *Consumer) Start(ctx context.Context) error {
	for _, s := range c.Subscriptions {
		s := s // capture
		qg := s.QueueGroup
		if qg == "" {
			qg = "metering"
		}
		bs, err := c.Sub.Subscribe(ctx, s.Subject, qg, func(ctx context.Context, msg bus.Message) error {
			return c.handle(s, msg)
		})
		if err != nil {
			return err
		}
		c.subs = append(c.subs, bs)
	}
	go c.runFlusher(ctx)
	return nil
}

// Close drains the bus subscriptions and force-flushes the buffer.
func (c *Consumer) Close(ctx context.Context) error {
	for _, s := range c.subs {
		_ = s.Drain()
	}
	return c.flush(ctx)
}

func (c *Consumer) handle(s Subscription, msg bus.Message) error {
	tenant := msg.Headers["tenant_id"]
	if tenant == "" {
		// Untenanted messages don't bill — silently drop. Counting these
		// would just produce noise in the audit trail.
		return nil
	}
	count := int64(1)
	if s.TokenField != "" {
		var body map[string]any
		if err := json.Unmarshal(msg.Data, &body); err == nil {
			if v, ok := body[s.TokenField]; ok {
				switch n := v.(type) {
				case float64:
					count = int64(n)
				case int64:
					count = n
				case int:
					count = int64(n)
				}
			}
		}
	}
	if count <= 0 {
		return nil
	}
	c.bump(bufKey{tenant: tenant, metric: s.Metric}, count)
	return nil
}

func (c *Consumer) bump(k bufKey, n int64) {
	c.mu.Lock()
	c.buf[k] = c.buf[k] + n
	overflowed := len(c.buf) > c.MaxBuffered
	c.mu.Unlock()
	if overflowed {
		_ = c.flush(context.Background())
	}
}

func (c *Consumer) runFlusher(ctx context.Context) {
	t := time.NewTicker(c.Flush)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := c.flush(ctx); err != nil && c.Logger != nil {
				c.Logger.WarnContext(ctx, "metering flush failed", slog.String("err", err.Error()))
			}
		}
	}
}

func (c *Consumer) flush(ctx context.Context) error {
	c.mu.Lock()
	if len(c.buf) == 0 {
		c.mu.Unlock()
		return nil
	}
	drain := c.buf
	c.buf = map[bufKey]int64{}
	c.mu.Unlock()
	for k, n := range drain {
		if err := c.Inc.Increment(ctx, k.tenant, k.metric, n); err != nil {
			// Roll the increment back into the buffer so we retry next tick.
			c.mu.Lock()
			c.buf[k] = c.buf[k] + n
			c.mu.Unlock()
			return err
		}
	}
	return nil
}
