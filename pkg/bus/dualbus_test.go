package bus_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/aegisvision/pkg/bus"
)

type recordingBus struct {
	name string
	mu   sync.Mutex
	got  []bus.Message
}

func (r *recordingBus) Publish(_ context.Context, m bus.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, m)
	return nil
}
func (r *recordingBus) Close() error { return nil }

func TestDualBus_PrefixRouting(t *testing.T) {
	kafka := &recordingBus{name: "kafka"}
	nats := &recordingBus{name: "nats"}
	db := &bus.DualBus{
		Routes: []bus.Route{
			{Prefix: "events.v1", Bus: kafka},
			{Prefix: "audit.v1", Bus: kafka},
			{Prefix: "frame.", Bus: nats},
			{Prefix: "operator.control", Bus: nats},
		},
		Default: nats,
	}
	send := []string{
		"events.v1", "audit.v1", "frame.descriptor.s1", "operator.control.p1", "unknown.topic",
	}
	for _, s := range send {
		_ = db.Publish(context.Background(), bus.Message{Subject: s, Data: []byte("x")})
	}
	if len(kafka.got) != 2 {
		t.Fatalf("kafka want 2, got %d", len(kafka.got))
	}
	if len(nats.got) != 3 {
		t.Fatalf("nats want 3, got %d (default fallback)", len(nats.got))
	}
}

func TestDualBus_NoRouteNoDefault(t *testing.T) {
	db := &bus.DualBus{Routes: []bus.Route{{Prefix: "x.", Bus: &recordingBus{}}}}
	err := db.Publish(context.Background(), bus.Message{Subject: "y.z"})
	if err == nil {
		t.Fatal("want error")
	}
	if !errors.Is(err, errors.New("bus: no route for subject y.z")) && err.Error() != "bus: no route for subject y.z" {
		t.Fatalf("err: %v", err)
	}
}
