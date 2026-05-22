package store_test

import (
	"testing"
	"time"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
	"github.com/aegisvision/services/event-service/internal/store"
)

func TestRing_RetentionAndListOrdering(t *testing.T) {
	r := store.New(3)
	for i := 0; i < 5; i++ {
		r.Append(&dataplanev1.Event{Id: name(i), TenantId: "t-1", StreamId: "s-1"})
	}
	got := r.List("t-1", "s-1", 10)
	if len(got) != 3 {
		t.Fatalf("want 3 retained, got %d", len(got))
	}
	// Most recent first.
	if got[0].GetId() != "e-4" || got[2].GetId() != "e-2" {
		t.Fatalf("unexpected order: %v", ids(got))
	}
}

func TestRing_SubscribeFilters(t *testing.T) {
	r := store.New(10)
	_, ch, cancel := r.Subscribe("t-1", "s-1", 8)
	defer cancel()

	r.Append(&dataplanev1.Event{TenantId: "t-1", StreamId: "s-1", Id: "match"})
	r.Append(&dataplanev1.Event{TenantId: "t-1", StreamId: "s-2", Id: "skip"})

	select {
	case ev := <-ch:
		if ev.GetId() != "match" {
			t.Fatalf("wanted 'match' first, got %q", ev.GetId())
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first event")
	}
	select {
	case ev := <-ch:
		t.Fatalf("did not expect 'skip', got %q", ev.GetId())
	case <-time.After(100 * time.Millisecond):
	}
}

func name(i int) string { return "e-" + string(rune('0'+i)) }
func ids(es []*dataplanev1.Event) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.GetId()
	}
	return out
}
