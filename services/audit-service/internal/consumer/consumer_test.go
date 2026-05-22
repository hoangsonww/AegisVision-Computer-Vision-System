package consumer_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/services/audit-service/internal/consumer"
)

type fakeStore struct {
	mu     sync.Mutex
	bodies [][]byte
	calls  atomic.Int64
}

func (f *fakeStore) AppendJSON(_ context.Context, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies = append(f.bodies, append([]byte(nil), body...))
	f.calls.Add(1)
	return nil
}

func TestConsumer_AppendsBusMessages(t *testing.T) {
	mem := bus.NewInMemory()
	fs := &fakeStore{}
	c := consumer.New(nil, mem, fs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"id":"r-1","at":"2026-05-21T00:00:00Z"}`)
	_ = mem.Publish(ctx, bus.Message{Subject: "audit.v1", Data: body})
	time.Sleep(50 * time.Millisecond)
	if fs.calls.Load() == 0 {
		t.Fatal("expected append")
	}
}

func TestConsumer_AcceptsWildcardCategoryPrefixes(t *testing.T) {
	mem := bus.NewInMemory()
	fs := &fakeStore{}
	c := consumer.New(nil, mem, fs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	_ = mem.Publish(ctx, bus.Message{Subject: "audit.policy-gate.v1", Data: []byte(`{}`)})
	time.Sleep(50 * time.Millisecond)
	if fs.calls.Load() < 1 {
		t.Fatalf("expected wildcard subscription to fire, calls=%d", fs.calls.Load())
	}
}
