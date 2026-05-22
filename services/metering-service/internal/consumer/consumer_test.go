package consumer_test

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/services/metering-service/internal/consumer"
)

type recorder struct {
	mu     sync.Mutex
	counts map[string]int64
	calls  atomic.Int64
}

func newRecorder() *recorder { return &recorder{counts: map[string]int64{}} }

func (r *recorder) Increment(_ context.Context, tenant, metric string, n int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[tenant+"|"+metric] += n
	r.calls.Add(1)
	return nil
}

func (r *recorder) Get(tenant, metric string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[tenant+"|"+metric]
}

func TestConsumer_CountsByOnePerMessage(t *testing.T) {
	mem := bus.NewInMemory()
	rec := newRecorder()
	c := consumer.New(nil, mem, rec)
	c.Flush = 50 * time.Millisecond
	c.Subscriptions = []consumer.Subscription{{Subject: "detections.v1", Metric: "detections"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		_ = mem.Publish(ctx, bus.Message{
			Subject: "detections.v1",
			Data:    []byte("{}"),
			Headers: map[string]string{"tenant_id": "t-1"},
		})
	}
	time.Sleep(200 * time.Millisecond)
	_ = c.Close(context.Background())
	if got := rec.Get("t-1", "detections"); got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
}

func TestConsumer_UsesTokenFieldForLLM(t *testing.T) {
	mem := bus.NewInMemory()
	rec := newRecorder()
	c := consumer.New(nil, mem, rec)
	c.Flush = 50 * time.Millisecond
	c.Subscriptions = []consumer.Subscription{{
		Subject: "llm.completed.v1", Metric: "llm_tokens", TokenField: "total_tokens",
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"total_tokens": 1200})
	_ = mem.Publish(ctx, bus.Message{
		Subject: "llm.completed.v1", Data: body,
		Headers: map[string]string{"tenant_id": "t-1"},
	})
	body2, _ := json.Marshal(map[string]any{"total_tokens": 300})
	_ = mem.Publish(ctx, bus.Message{
		Subject: "llm.completed.v1", Data: body2,
		Headers: map[string]string{"tenant_id": "t-1"},
	})
	time.Sleep(200 * time.Millisecond)
	_ = c.Close(context.Background())
	if got := rec.Get("t-1", "llm_tokens"); got != 1500 {
		t.Fatalf("expected 1500 tokens, got %d", got)
	}
}

func TestConsumer_DropsUntenantedMessages(t *testing.T) {
	mem := bus.NewInMemory()
	rec := newRecorder()
	c := consumer.New(nil, mem, rec)
	c.Flush = 50 * time.Millisecond
	c.Subscriptions = []consumer.Subscription{{Subject: "detections.v1", Metric: "detections"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = c.Start(ctx)
	_ = mem.Publish(ctx, bus.Message{Subject: "detections.v1", Data: []byte("{}")})
	time.Sleep(150 * time.Millisecond)
	_ = c.Close(context.Background())
	if rec.calls.Load() != 0 {
		t.Fatalf("untenanted message produced an increment: %d", rec.calls.Load())
	}
}

func TestConsumer_BatchesIncrements(t *testing.T) {
	mem := bus.NewInMemory()
	rec := newRecorder()
	c := consumer.New(nil, mem, rec)
	c.Flush = 100 * time.Millisecond
	c.Subscriptions = []consumer.Subscription{{Subject: "detections.v1", Metric: "detections"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = c.Start(ctx)
	for i := 0; i < 50; i++ {
		_ = mem.Publish(ctx, bus.Message{
			Subject: "detections.v1", Data: []byte("{}"),
			Headers: map[string]string{"tenant_id": "t-1"},
		})
	}
	time.Sleep(200 * time.Millisecond)
	_ = c.Close(context.Background())
	// All 50 detections should land in a single increment call (or two if a
	// flush happened mid-burst) — strictly fewer than 50.
	if rec.calls.Load() >= 50 {
		t.Fatalf("not batched: %d calls for 50 events", rec.calls.Load())
	}
	if got := rec.Get("t-1", "detections"); got != 50 {
		t.Fatalf("expected 50, got %d", got)
	}
}
