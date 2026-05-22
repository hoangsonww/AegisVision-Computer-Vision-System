package queue_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aegisvision/services/edge-gateway/internal/queue"
)

func TestQueue_PutPeekAck(t *testing.T) {
	q, err := queue.New(t.TempDir(), 100, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := q.Put("events.v1", []byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if q.Depth() != 5 {
		t.Fatalf("depth: %d", q.Depth())
	}
	got := q.Peek(3)
	if len(got) != 3 || got[0].Seq != 1 || got[2].Seq != 3 {
		t.Fatalf("peek: %+v", got)
	}
	if err := q.AckThrough(3); err != nil {
		t.Fatal(err)
	}
	if q.Depth() != 2 {
		t.Fatalf("after ack: %d", q.Depth())
	}
}

func TestQueue_ReplayOnRestart(t *testing.T) {
	dir := t.TempDir()
	q1, _ := queue.New(dir, 100, 1<<20)
	for i := 0; i < 3; i++ {
		_, _ = q1.Put("events.v1", []byte{byte(i)})
	}
	q2, err := queue.New(dir, 100, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if q2.Depth() != 3 {
		t.Fatalf("replay: want 3, got %d", q2.Depth())
	}
	got := q2.Peek(10)
	if got[0].Seq != 1 || got[2].Seq != 3 {
		t.Fatalf("replay order: %+v", got)
	}
}

func TestQueue_DrainAckPattern(t *testing.T) {
	q, _ := queue.New(t.TempDir(), 100, 1<<20)
	for i := 0; i < 5; i++ {
		_, _ = q.Put("x", []byte{byte(i)})
	}
	seen := 0
	err := q.Drain(context.Background(), func(_ context.Context, _ queue.Item) error {
		seen++
		if seen == 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected drain to return error")
	}
	// First 2 should be ack'd; 3 onwards still in queue.
	if q.Depth() != 3 {
		t.Fatalf("after partial drain: %d", q.Depth())
	}
}

func TestQueue_CapDropsOldest(t *testing.T) {
	q, _ := queue.New(t.TempDir(), 3, 1<<20)
	for i := 0; i < 5; i++ {
		_, _ = q.Put("x", []byte{byte(i)})
	}
	if q.Depth() != 3 {
		t.Fatalf("cap not enforced: %d", q.Depth())
	}
	got := q.Peek(10)
	// Oldest seqs (1, 2) dropped; 3..5 retained.
	if got[0].Seq != 3 {
		t.Fatalf("oldest not dropped: first seq=%d", got[0].Seq)
	}
}
