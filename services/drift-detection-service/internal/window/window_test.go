package window_test

import (
	"math"
	"testing"
	"time"

	"github.com/aegisvision/services/drift-detection-service/internal/window"
)

func TestRolling_NormalizesToOne(t *testing.T) {
	w := window.New(60)
	for i := 0; i < 30; i++ {
		w.Observe("person")
	}
	for i := 0; i < 10; i++ {
		w.Observe("vehicle")
	}
	snap := w.Snapshot()
	if snap.Samples != 40 {
		t.Fatalf("samples: %d", snap.Samples)
	}
	sum := 0.0
	for _, v := range snap.Classes {
		sum += v
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Fatalf("not normalized: %f", sum)
	}
	if math.Abs(snap.Classes["person"]-0.75) > 1e-6 {
		t.Fatalf("person ratio: %f", snap.Classes["person"])
	}
}

func TestRolling_PrunesOldBuckets(t *testing.T) {
	w := window.New(10)
	t0 := time.Now()
	now := t0
	w.SetNow(func() time.Time { return now })
	for i := 0; i < 5; i++ {
		w.Observe("a")
	}
	// Move forward 20 seconds.
	now = t0.Add(20 * time.Second)
	for i := 0; i < 3; i++ {
		w.Observe("b")
	}
	snap := w.Snapshot()
	if snap.Samples != 3 {
		t.Fatalf("expected only 3 samples after prune, got %d", snap.Samples)
	}
	if _, ok := snap.Classes["a"]; ok {
		t.Fatal("old class 'a' should have been pruned")
	}
}
