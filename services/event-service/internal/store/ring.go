// Bounded in-memory ring buffer of recent events. The walking skeleton
// uses this as the read-side store; Phase 2 replaces with a ClickHouse-
// backed implementation (ADR-0002). The interface is intentionally narrow.
package store

import (
	"sync"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

// Store retains the most recent N events and supports per-stream listing.
type Store interface {
	Append(ev *dataplanev1.Event)
	List(tenantID, streamID string, limit int) []*dataplanev1.Event
	Subscribe(tenantID, streamID string, capacity int) (id int, ch <-chan *dataplanev1.Event, cancel func())
}

// Ring is a fixed-capacity FIFO of *Event with per-stream filtering and
// live subscriber fan-out.
type Ring struct {
	mu       sync.Mutex
	events   []*dataplanev1.Event
	capacity int
	subs     map[int]*subscription
	nextSub  int
}

type subscription struct {
	tenantID string
	streamID string
	ch       chan *dataplanev1.Event
}

// New returns a Ring with the given capacity. Capacity = 0 disables
// retention; the store still fans out to live subscribers.
func New(capacity int) *Ring {
	return &Ring{capacity: capacity, subs: map[int]*subscription{}}
}

// Append stores the event (subject to capacity) and forwards to every
// matching live subscriber. Drops on a full subscriber channel to keep
// slow consumers from blocking the bus consumer.
func (r *Ring) Append(ev *dataplanev1.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.capacity > 0 {
		r.events = append(r.events, ev)
		if len(r.events) > r.capacity {
			r.events = r.events[len(r.events)-r.capacity:]
		}
	}
	for _, s := range r.subs {
		if s.tenantID != "" && ev.GetTenantId() != s.tenantID {
			continue
		}
		if s.streamID != "" && ev.GetStreamId() != s.streamID {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			// Drop on full — surfaces as a metric in the wrapping service.
		}
	}
}

// List returns the most recent retained events matching the filter.
func (r *Ring) List(tenantID, streamID string, limit int) []*dataplanev1.Event {
	if limit <= 0 {
		limit = 100
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*dataplanev1.Event, 0, limit)
	for i := len(r.events) - 1; i >= 0 && len(out) < limit; i-- {
		ev := r.events[i]
		if tenantID != "" && ev.GetTenantId() != tenantID {
			continue
		}
		if streamID != "" && ev.GetStreamId() != streamID {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// Subscribe returns a channel that receives matching events as they arrive.
// The cancel function detaches the subscriber and closes the channel.
func (r *Ring) Subscribe(tenantID, streamID string, capacity int) (int, <-chan *dataplanev1.Event, func()) {
	if capacity <= 0 {
		capacity = 64
	}
	r.mu.Lock()
	id := r.nextSub
	r.nextSub++
	s := &subscription{tenantID: tenantID, streamID: streamID, ch: make(chan *dataplanev1.Event, capacity)}
	r.subs[id] = s
	r.mu.Unlock()
	cancel := func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if existing, ok := r.subs[id]; ok {
			delete(r.subs, id)
			close(existing.ch)
		}
	}
	return id, s.ch, cancel
}
