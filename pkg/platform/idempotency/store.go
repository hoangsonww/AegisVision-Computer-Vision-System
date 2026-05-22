// Package idempotency provides the in-process cache backing the
// Idempotency-Key middleware. Per the architecture doc, every mutating
// endpoint MUST accept Idempotency-Key, and replays within a 24-hour window
// must return the cached response.
//
// This implementation is in-memory. Production deployments substitute a Redis
// or PostgreSQL-backed Store (the Store interface is satisfied by both).
package idempotency

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Entry is the cached response stored against an idempotency key.
type Entry struct {
	Status      int
	ContentType string
	Body        []byte
	// Fingerprint of the request body; if a later replay carries a different
	// body under the same key, return ErrConflict so the gateway can return
	// 409 (Conflict) rather than serve a stale response.
	BodyFingerprint string
	StoredAt        time.Time
	ExpiresAt       time.Time
}

// ErrConflict signals that the same key was reused with a different body.
var ErrConflict = errors.New("idempotency: body fingerprint mismatch")

// Store abstracts the persistence layer.
type Store interface {
	Get(ctx context.Context, tenantID, key string) (Entry, bool, error)
	Put(ctx context.Context, tenantID, key string, e Entry) error
}

// MemoryStore is an in-process Store suitable for local dev and tests.
type MemoryStore struct {
	mu       sync.Mutex
	entries  map[string]Entry
	now      func() time.Time
	maxItems int
}

// NewMemoryStore constructs a MemoryStore. maxItems bounds the cache size to
// keep a runaway test from OOM-ing a process; entries are evicted in
// expiry-first order when the bound is reached.
func NewMemoryStore(maxItems int) *MemoryStore {
	if maxItems <= 0 {
		maxItems = 10_000
	}
	return &MemoryStore{
		entries:  make(map[string]Entry),
		now:      time.Now,
		maxItems: maxItems,
	}
}

func (m *MemoryStore) key(tenantID, key string) string { return tenantID + "|" + key }

func (m *MemoryStore) Get(_ context.Context, tenantID, key string) (Entry, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[m.key(tenantID, key)]
	if !ok {
		return Entry{}, false, nil
	}
	if m.now().After(e.ExpiresAt) {
		delete(m.entries, m.key(tenantID, key))
		return Entry{}, false, nil
	}
	return e, true, nil
}

func (m *MemoryStore) Put(_ context.Context, tenantID, key string, e Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.entries) >= m.maxItems {
		m.evictOneLocked()
	}
	m.entries[m.key(tenantID, key)] = e
	return nil
}

// evictOneLocked removes the entry with the earliest expiry. O(n) but only
// runs when the cap is hit and the store is meant for local dev.
func (m *MemoryStore) evictOneLocked() {
	var victim string
	var oldest time.Time
	for k, v := range m.entries {
		if victim == "" || v.ExpiresAt.Before(oldest) {
			victim = k
			oldest = v.ExpiresAt
		}
	}
	if victim != "" {
		delete(m.entries, victim)
	}
}
