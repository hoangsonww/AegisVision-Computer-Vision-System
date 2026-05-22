// Per-(tenant, model) rolling window of observed class counts. Detections
// stream in; the window keeps the last N seconds' worth of counts and
// surfaces a normalized distribution on demand.
//
// The window is a simple ring of (bucket-start → counts). Each bucket is
// 1 second wide; total window width is policy-driven (typically 5–15 min).
// This is sufficient resolution for drift detection without going to
// percentile-data-structures territory.
package window

import (
	"sync"
	"time"

	"github.com/aegisvision/pkg/intelligence"
)

// Rolling keeps a sliding window of class counts.
type Rolling struct {
	mu        sync.Mutex
	now       func() time.Time
	widthSec  int
	buckets   map[int64]map[string]int64 // bucket-start (sec) → class → count
	totalsCache int64
}

// New constructs a window with the given total width (seconds).
func New(widthSeconds int) *Rolling {
	if widthSeconds <= 0 {
		widthSeconds = 900
	}
	return &Rolling{
		now: time.Now, widthSec: widthSeconds,
		buckets: map[int64]map[string]int64{},
	}
}

// SetNow is exposed for tests.
func (r *Rolling) SetNow(f func() time.Time) { r.mu.Lock(); r.now = f; r.mu.Unlock() }

// Observe records one detection for the given class at the current time.
func (r *Rolling) Observe(class string) {
	if class == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now().Unix()
	b, ok := r.buckets[now]
	if !ok {
		b = map[string]int64{}
		r.buckets[now] = b
	}
	b[class]++
}

// Snapshot returns the current normalized distribution + sample count
// over the window. Buckets older than width are pruned.
func (r *Rolling) Snapshot() intelligence.Distribution {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now().Unix()
	cutoff := now - int64(r.widthSec)
	for sec := range r.buckets {
		if sec < cutoff {
			delete(r.buckets, sec)
		}
	}
	classes := map[string]float64{}
	var total int64
	for _, b := range r.buckets {
		for c, n := range b {
			classes[c] += float64(n)
			total += n
		}
	}
	if total > 0 {
		for c := range classes {
			classes[c] = classes[c] / float64(total)
		}
	}
	return intelligence.Distribution{
		Classes:     classes,
		Samples:     total,
		WindowStart: time.Unix(cutoff, 0).UTC(),
		WindowEnd:   time.Unix(now, 0).UTC(),
	}
}
