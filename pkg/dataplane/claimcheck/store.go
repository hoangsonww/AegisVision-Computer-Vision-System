// Package claimcheck holds the in-process payload store that backs
// FrameDescriptor.payload.KIND_SHARED_MEMORY.
//
// The architecture mandates that pixels never travel on the bus (ADR-0008).
// In production this is realized by hugepages-backed shared-memory rings or
// CUDA-IPC; for the walking skeleton we implement an in-process ring whose
// observable contract is the same: descriptor on the bus, pixels behind the
// locator. When real on-node operators land, the ring graduates to an
// mmap-backed implementation without changing operator code.
package claimcheck

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Ring is a fixed-capacity slot ring that holds the most recent frames.
// Producers (ingest operator) write into a free slot; consumers (decode,
// inference, ...) read by locator. Old frames are overwritten silently —
// matching real shared-memory ring semantics under sustained load.
type Ring struct {
	name string
	mu   sync.RWMutex
	cap  int
	next atomic.Uint64
	// slots[i] holds the most recent payload that landed in slot i; nil if
	// the slot has never been written.
	slots [][]byte
	// generation[i] is incremented every time slot i is rewritten so a
	// reader can detect stale references.
	generation []uint64
}

// NewRing creates a Ring with the given name and capacity. Capacity must be
// > 0; production sizing typically holds ~30 frames per stream (one second
// at 30fps) so a slow consumer can briefly fall behind without dropping.
func NewRing(name string, capacity int) (*Ring, error) {
	if capacity <= 0 {
		return nil, errors.New("claimcheck: capacity must be positive")
	}
	return &Ring{
		name:       name,
		cap:        capacity,
		slots:      make([][]byte, capacity),
		generation: make([]uint64, capacity),
	}, nil
}

// Locator identifies one written slot. Encoded into the FrameDescriptor's
// PayloadLocation.locator field as "<ring>:<slot>:<gen>".
type Locator struct {
	Ring string
	Slot int
	Gen  uint64
}

// Encode returns the wire format of the locator.
func (l Locator) Encode() string {
	return fmt.Sprintf("%s:%d:%d", l.Ring, l.Slot, l.Gen)
}

// ParseLocator decodes the wire format. Returns an error if malformed.
func ParseLocator(s string) (Locator, error) {
	var l Locator
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return l, fmt.Errorf("claimcheck: parse locator %q: want <ring>:<slot>:<gen>", s)
	}
	slot, err := strconv.Atoi(parts[1])
	if err != nil {
		return l, fmt.Errorf("claimcheck: parse locator %q: slot: %w", s, err)
	}
	gen, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return l, fmt.Errorf("claimcheck: parse locator %q: gen: %w", s, err)
	}
	return Locator{Ring: parts[0], Slot: slot, Gen: gen}, nil
}

// Put writes payload into the next slot and returns its locator. Concurrent
// Puts are serialized through the next pointer; concurrent Get on a slot
// being rewritten will observe either the old or new value, never a torn
// read (the slot itself is replaced atomically by Go's slice semantics).
func (r *Ring) Put(payload []byte) Locator {
	idx := int(r.next.Add(1)-1) % r.cap
	r.mu.Lock()
	r.slots[idx] = payload
	r.generation[idx]++
	gen := r.generation[idx]
	r.mu.Unlock()
	return Locator{Ring: r.name, Slot: idx, Gen: gen}
}

// Get returns the payload at the locator, or ErrStale if it has been
// overwritten since.
func (r *Ring) Get(l Locator) ([]byte, error) {
	if l.Slot < 0 || l.Slot >= r.cap {
		return nil, errors.New("claimcheck: slot out of range")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.generation[l.Slot] != l.Gen {
		return nil, ErrStale
	}
	if r.slots[l.Slot] == nil {
		return nil, ErrEmpty
	}
	// Copy so callers can't mutate ring memory. In a hugepages/mmap impl
	// this would be a zero-copy view; the skeleton honors the abstraction
	// rather than the optimization.
	out := make([]byte, len(r.slots[l.Slot]))
	copy(out, r.slots[l.Slot])
	return out, nil
}

// Errors callers can branch on.
var (
	ErrStale = errors.New("claimcheck: locator references an overwritten slot")
	ErrEmpty = errors.New("claimcheck: slot has never been written")
)

// Registry is the per-process directory of named Rings. The dataplane-runner
// keeps one Registry and hands it to each operator at construction.
type Registry struct {
	mu    sync.RWMutex
	rings map[string]*Ring
}

func NewRegistry() *Registry { return &Registry{rings: map[string]*Ring{}} }

// Ensure returns the ring of the given name, creating it with the supplied
// capacity if absent.
func (r *Registry) Ensure(name string, capacity int) (*Ring, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ring, ok := r.rings[name]; ok {
		return ring, nil
	}
	ring, err := NewRing(name, capacity)
	if err != nil {
		return nil, err
	}
	r.rings[name] = ring
	return ring, nil
}

// Lookup returns the named ring or nil.
func (r *Registry) Lookup(name string) *Ring {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rings[name]
}
