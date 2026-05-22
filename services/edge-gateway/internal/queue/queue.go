// Package queue is the edge-gateway's store-and-forward layer.
//
// Per ADR-0005 + the architecture doc §14, edge sites run a reduced operator
// set and forward results (detections, events, health) back to core. WAN
// links are unreliable; the queue absorbs disconnects up to its disk-backed
// capacity and replays on reconnect — at-least-once delivery with a
// sequence number so the core deduplicates.
package queue

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Item is a single queued message.
type Item struct {
	Seq     uint64
	Subject string
	Data    []byte
	AddedAt time.Time
}

// Queue is a disk-backed FIFO. Items are persisted as numbered files; on
// boot we scan the directory and rebuild the in-memory ordered list.
type Queue struct {
	mu        sync.Mutex
	dir       string
	next      uint64
	items     []Item
	maxItems  int
	maxBytes  int64
	curBytes  int64
}

// New opens a queue rooted at dir. maxItems and maxBytes cap the queue;
// older items are dropped (with a logged WARN) when the cap is exceeded.
func New(dir string, maxItems int, maxBytes int64) (*Queue, error) {
	if dir == "" {
		return nil, errors.New("queue: dir required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if maxItems <= 0 {
		maxItems = 100_000
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 30 // 1 GiB
	}
	q := &Queue{dir: dir, maxItems: maxItems, maxBytes: maxBytes}
	if err := q.replay(); err != nil {
		return nil, err
	}
	return q, nil
}

// Put appends a message. Returns the assigned sequence number.
func (q *Queue) Put(subject string, data []byte) (uint64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.next++
	it := Item{Seq: q.next, Subject: subject, Data: data, AddedAt: time.Now().UTC()}
	if err := q.persist(it); err != nil {
		q.next-- // roll back
		return 0, err
	}
	q.items = append(q.items, it)
	q.curBytes += int64(len(data))
	q.dropExcessLocked()
	return it.Seq, nil
}

// Peek returns up to n oldest items without removing them. Use this to
// build a batch for the bus consumer.
func (q *Queue) Peek(n int) []Item {
	if n <= 0 {
		n = 32
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if n > len(q.items) {
		n = len(q.items)
	}
	out := make([]Item, n)
	copy(out, q.items[:n])
	return out
}

// AckThrough removes every item with seq <= upTo. Use after a successful
// publish to the core.
func (q *Queue) AckThrough(upTo uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	idx := sort.Search(len(q.items), func(i int) bool { return q.items[i].Seq > upTo })
	for i := 0; i < idx; i++ {
		_ = os.Remove(q.filenameFor(q.items[i].Seq))
		q.curBytes -= int64(len(q.items[i].Data))
	}
	q.items = q.items[idx:]
	return nil
}

// Depth returns the current item count.
func (q *Queue) Depth() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Drain executes fn for each pending item in order, ack'ing only after fn
// returns nil. Used by the forwarder when network is back.
func (q *Queue) Drain(ctx context.Context, fn func(context.Context, Item) error) error {
	for {
		batch := q.Peek(64)
		if len(batch) == 0 {
			return nil
		}
		var lastAck uint64
		for _, it := range batch {
			if ctx.Err() != nil {
				_ = q.AckThrough(lastAck)
				return ctx.Err()
			}
			if err := fn(ctx, it); err != nil {
				_ = q.AckThrough(lastAck)
				return err
			}
			lastAck = it.Seq
		}
		_ = q.AckThrough(lastAck)
	}
}

// --- persistence ---

func (q *Queue) filenameFor(seq uint64) string {
	return filepath.Join(q.dir, fmt.Sprintf("%020d.itm", seq))
}

func (q *Queue) persist(it Item) error {
	f, err := os.OpenFile(q.filenameFor(it.Seq), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	// Format: [u16 subjectLen][subject][u32 dataLen][data].
	if err := binary.Write(f, binary.BigEndian, uint16(len(it.Subject))); err != nil {
		return err
	}
	if _, err := f.Write([]byte(it.Subject)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.BigEndian, uint32(len(it.Data))); err != nil {
		return err
	}
	if _, err := f.Write(it.Data); err != nil {
		return err
	}
	return f.Sync()
}

func (q *Queue) replay() error {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		var seq uint64
		if _, err := fmt.Sscanf(n, "%020d.itm", &seq); err != nil {
			continue
		}
		body, err := os.ReadFile(filepath.Join(q.dir, n))
		if err != nil {
			continue
		}
		if len(body) < 6 {
			continue
		}
		sl := binary.BigEndian.Uint16(body[:2])
		if int(2+sl+4) > len(body) {
			continue
		}
		subj := string(body[2 : 2+sl])
		dl := binary.BigEndian.Uint32(body[2+sl : 6+sl])
		data := body[6+sl : 6+sl+uint16(dl)]
		q.items = append(q.items, Item{Seq: seq, Subject: subj, Data: data})
		q.curBytes += int64(len(data))
		if seq > q.next {
			q.next = seq
		}
	}
	return nil
}

func (q *Queue) dropExcessLocked() {
	for len(q.items) > q.maxItems || q.curBytes > q.maxBytes {
		if len(q.items) == 0 {
			return
		}
		victim := q.items[0]
		q.items = q.items[1:]
		q.curBytes -= int64(len(victim.Data))
		_ = os.Remove(q.filenameFor(victim.Seq))
	}
}
