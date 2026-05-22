// InMemoryBus is a goroutine-safe, in-process bus suitable for unit and
// integration tests. It supports wildcard subject matching ("foo.*" matches
// "foo.bar" but not "foo.bar.baz") so tests can assert "any event for tenant
// X" without coupling to one specific subject.
package bus

import (
	"context"
	"strings"
	"sync"
)

// InMemoryBus implements Publisher and Subscriber against an in-process
// fan-out.
type InMemoryBus struct {
	mu          sync.RWMutex
	subscribers []*memSub
}

// NewInMemory returns a fresh in-process bus.
func NewInMemory() *InMemoryBus { return &InMemoryBus{} }

type memSub struct {
	subject    string
	queueGroup string
	handler    Handler
	closed     bool
	bus        *InMemoryBus
}

// Publish fans out the message to every matching subscriber. Queue-group
// subscribers receive exactly one copy across the group.
func (b *InMemoryBus) Publish(ctx context.Context, msg Message) error {
	b.mu.RLock()
	subs := make([]*memSub, 0, len(b.subscribers))
	for _, s := range b.subscribers {
		if s.closed {
			continue
		}
		if !subjectMatch(s.subject, msg.Subject) {
			continue
		}
		subs = append(subs, s)
	}
	b.mu.RUnlock()

	// Group by queue group; pick the first member of each group.
	delivered := make(map[string]bool)
	for _, s := range subs {
		key := s.queueGroup
		if key == "" {
			// Fanout — every subscriber gets a copy.
			_ = s.handler(ctx, msg)
			continue
		}
		if delivered[key] {
			continue
		}
		delivered[key] = true
		_ = s.handler(ctx, msg)
	}
	return nil
}

// Subscribe registers a handler for subjects matching pattern.
func (b *InMemoryBus) Subscribe(_ context.Context, subject, queueGroup string, h Handler) (Subscription, error) {
	s := &memSub{subject: subject, queueGroup: queueGroup, handler: h, bus: b}
	b.mu.Lock()
	b.subscribers = append(b.subscribers, s)
	b.mu.Unlock()
	return s, nil
}

// Close drops every subscription.
func (b *InMemoryBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.subscribers {
		s.closed = true
	}
	b.subscribers = nil
	return nil
}

func (s *memSub) Drain() error       { return s.Unsubscribe() }
func (s *memSub) Unsubscribe() error { s.closed = true; return nil }

// subjectMatch supports a small subset of NATS subject wildcards:
//
//	*  matches one token
//	>  matches one-or-more trailing tokens
func subjectMatch(pattern, subject string) bool {
	if pattern == subject {
		return true
	}
	ps := strings.Split(pattern, ".")
	ss := strings.Split(subject, ".")
	for i, tok := range ps {
		if tok == ">" {
			return i < len(ss)
		}
		if i >= len(ss) {
			return false
		}
		if tok == "*" || tok == ss[i] {
			continue
		}
		return false
	}
	return len(ps) == len(ss)
}
