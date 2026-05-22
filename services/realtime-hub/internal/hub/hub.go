// Package hub fans events out to many WebSocket subscribers. The realtime-hub
// runs as a separate service (vs SSE in event-service) so the platform can
// scale WebSocket connection count independently of the durable read path.
//
// Backpressure semantics: each subscriber owns a bounded outbound channel.
// On full, the hub drops the message for that subscriber and increments a
// `aegis_realtime_dropped_total{reason="slow_consumer"}` counter — clients
// that can't keep up don't slow others.
package hub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

// Filter narrows which events a subscriber receives.
type Filter struct {
	TenantID string
	StreamID string // optional
}

// Subscriber is one WebSocket connection's outbound view.
type Subscriber struct {
	id     uint64
	filter Filter
	ch     chan *dataplanev1.Event
	closed atomic.Bool
}

// Out returns the subscriber's outbound channel.
func (s *Subscriber) Out() <-chan *dataplanev1.Event { return s.ch }
func (s *Subscriber) ID() uint64                     { return s.id }

// Hub is goroutine-safe.
type Hub struct {
	mu      sync.RWMutex
	subs    map[uint64]*Subscriber
	next    atomic.Uint64
	dropped atomic.Uint64
}

func New() *Hub { return &Hub{subs: map[uint64]*Subscriber{}} }

// Subscribe registers a new subscriber. Capacity bounds the outbound channel.
func (h *Hub) Subscribe(filter Filter, capacity int) *Subscriber {
	if capacity <= 0 {
		capacity = 256
	}
	s := &Subscriber{
		id:     h.next.Add(1),
		filter: filter,
		ch:     make(chan *dataplanev1.Event, capacity),
	}
	h.mu.Lock()
	h.subs[s.id] = s
	h.mu.Unlock()
	return s
}

// Unsubscribe disconnects a subscriber and closes its channel.
func (h *Hub) Unsubscribe(s *Subscriber) {
	if s == nil || !s.closed.CompareAndSwap(false, true) {
		return
	}
	h.mu.Lock()
	delete(h.subs, s.id)
	h.mu.Unlock()
	close(s.ch)
}

// Publish fans out to every matching subscriber. Drops on full per subscriber.
func (h *Hub) Publish(_ context.Context, ev *dataplanev1.Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subs {
		if s.closed.Load() {
			continue
		}
		if s.filter.TenantID != "" && ev.GetTenantId() != s.filter.TenantID {
			continue
		}
		if s.filter.StreamID != "" && ev.GetStreamId() != s.filter.StreamID {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			h.dropped.Add(1)
		}
	}
}

// Dropped returns the running count of slow-consumer drops since boot.
func (h *Hub) Dropped() uint64 { return h.dropped.Load() }

// Subscribers returns the current count.
func (h *Hub) Subscribers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// ErrClosed is returned by helper sends to a closed subscriber.
var ErrClosed = errors.New("hub: subscriber closed")
