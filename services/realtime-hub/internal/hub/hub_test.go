package hub_test

import (
	"context"
	"testing"
	"time"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
	"github.com/aegisvision/services/realtime-hub/internal/hub"
)

func TestHub_FanoutWithFilter(t *testing.T) {
	h := hub.New()
	a := h.Subscribe(hub.Filter{TenantID: "t-1"}, 8)
	b := h.Subscribe(hub.Filter{TenantID: "t-1", StreamID: "s-x"}, 8)
	c := h.Subscribe(hub.Filter{TenantID: "t-2"}, 8)
	defer h.Unsubscribe(a)
	defer h.Unsubscribe(b)
	defer h.Unsubscribe(c)

	h.Publish(context.Background(), &dataplanev1.Event{TenantId: "t-1", StreamId: "s-x"})
	h.Publish(context.Background(), &dataplanev1.Event{TenantId: "t-1", StreamId: "s-y"})

	expect := func(s *hub.Subscriber, want int) {
		t.Helper()
		got := 0
		for got < want {
			select {
			case <-s.Out():
				got++
			case <-time.After(200 * time.Millisecond):
				t.Fatalf("sub %d: want %d, got %d", s.ID(), want, got)
			}
		}
		select {
		case <-s.Out():
			t.Fatalf("sub %d: unexpected extra message", s.ID())
		case <-time.After(50 * time.Millisecond):
		}
	}
	expect(a, 2)
	expect(b, 1)
	// c is on a different tenant; must see nothing.
	select {
	case <-c.Out():
		t.Fatal("c should not have received cross-tenant event")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHub_DropsOnSlowConsumer(t *testing.T) {
	h := hub.New()
	s := h.Subscribe(hub.Filter{TenantID: "t-1"}, 2)
	defer h.Unsubscribe(s)
	for i := 0; i < 50; i++ {
		h.Publish(context.Background(), &dataplanev1.Event{TenantId: "t-1"})
	}
	if h.Dropped() == 0 {
		t.Fatal("expected drops on slow consumer")
	}
}
