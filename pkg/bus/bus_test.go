package bus_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aegisvision/pkg/bus"
)

func TestInMemoryBus_PublishFanout(t *testing.T) {
	b := bus.NewInMemory()
	defer b.Close()

	var mu sync.Mutex
	received := []string{}
	_, err := b.Subscribe(context.Background(), "events.v1", "", func(_ context.Context, m bus.Message) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, string(m.Data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Publish(context.Background(), bus.Message{Subject: "events.v1", Data: []byte("a")})
	_ = b.Publish(context.Background(), bus.Message{Subject: "events.v1", Data: []byte("b")})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("want 2, got %d", len(received))
	}
}

func TestInMemoryBus_QueueGroupSingleDelivery(t *testing.T) {
	b := bus.NewInMemory()
	defer b.Close()

	var a, c int
	_, _ = b.Subscribe(context.Background(), "events.v1", "group-1", func(context.Context, bus.Message) error { a++; return nil })
	_, _ = b.Subscribe(context.Background(), "events.v1", "group-1", func(context.Context, bus.Message) error { c++; return nil })

	for i := 0; i < 10; i++ {
		_ = b.Publish(context.Background(), bus.Message{Subject: "events.v1"})
	}
	if a+c != 10 {
		t.Fatalf("queue-group total: want 10, got %d (a=%d c=%d)", a+c, a, c)
	}
	// First subscriber in the group should have received all 10; deterministic
	// because of the order in subscribers slice, but the contract only
	// requires exactly-one delivery per group per publish.
}

func TestInMemoryBus_WildcardMatch(t *testing.T) {
	b := bus.NewInMemory()
	defer b.Close()
	var hits int
	_, _ = b.Subscribe(context.Background(), "frame.descriptor.*", "", func(context.Context, bus.Message) error { hits++; return nil })
	_ = b.Publish(context.Background(), bus.Message{Subject: "frame.descriptor.stream-1"})
	_ = b.Publish(context.Background(), bus.Message{Subject: "frame.descriptor.stream-2"})
	_ = b.Publish(context.Background(), bus.Message{Subject: "events.v1"})
	if hits != 2 {
		t.Fatalf("want 2 hits, got %d", hits)
	}
}

func TestNATS_RoundTrip_AgainstEmbeddedServer(t *testing.T) {
	url, srv, err := bus.EmbeddedServer()
	if err != nil {
		t.Skipf("embedded NATS not available: %v", err)
	}
	defer srv.Shutdown()

	pub, err := bus.Connect(url)
	if err != nil {
		t.Fatalf("publisher connect: %v", err)
	}
	defer pub.Close()

	sub, err := bus.Connect(url)
	if err != nil {
		t.Fatalf("subscriber connect: %v", err)
	}
	defer sub.Close()

	got := make(chan string, 4)
	subscription, err := sub.Subscribe(context.Background(), "events.v1", "", func(_ context.Context, m bus.Message) error {
		got <- string(m.Data)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer subscription.Unsubscribe()
	// Force the subscription to propagate to the server before publishing.
	if err := sub.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if err := pub.Publish(context.Background(), bus.Message{Subject: "events.v1", Data: []byte("ping")}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case msg := <-got:
		if msg != "ping" {
			t.Fatalf("want ping, got %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for delivery")
	}
}
