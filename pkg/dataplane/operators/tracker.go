// The tracker operator assigns stable track IDs to detections across
// frames. Phase 2 brings ByteTrack / BoT-SORT in C++; the skeleton ships
// a small IOU-greedy tracker that is correct enough for one moving target
// and exercises the same data flow.
package operators

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/aegisvision/pkg/dataplane"
	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
)

// TrackerConfig allows tuning of the association threshold; the default
// IOU of 0.25 keeps tracks stable across a fast-moving target.
type TrackerConfig struct {
	IOUThreshold float32
	// MaxMissing is the number of consecutive frames a track may go
	// undetected before it is closed. Defaults to 30 (~1s at 30fps).
	MaxMissing int
}

type trackState struct {
	id         string
	classLabel string
	lastBox    *dataplanev1.BoundingBox
	missed     int
}

type Tracker struct {
	cfg    TrackerConfig
	tracks []*trackState
}

func NewTracker(cfg TrackerConfig) *Tracker {
	if cfg.IOUThreshold <= 0 {
		cfg.IOUThreshold = 0.25
	}
	if cfg.MaxMissing <= 0 {
		cfg.MaxMissing = 30
	}
	return &Tracker{cfg: cfg}
}

func (t *Tracker) Name() string { return "tracker" }

func (t *Tracker) Run(ctx context.Context, in <-chan *dataplane.FrameEnvelope, out chan<- *dataplane.FrameEnvelope) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case env, ok := <-in:
			if !ok {
				return dataplane.ErrUpstreamClosed
			}
			t.assign(env)
			select {
			case <-ctx.Done():
				return nil
			case out <- env:
			}
		}
	}
}

func (t *Tracker) assign(env *dataplane.FrameEnvelope) {
	used := make([]bool, len(t.tracks))
	for di, det := range env.Detections {
		bestIdx := -1
		var bestIOU float32
		for ti, tr := range t.tracks {
			if used[ti] {
				continue
			}
			if tr.classLabel != det.GetClassLabel() {
				continue
			}
			iou := iou(tr.lastBox, det.GetBbox())
			if iou > bestIOU && iou >= t.cfg.IOUThreshold {
				bestIOU = iou
				bestIdx = ti
			}
		}
		if bestIdx >= 0 {
			used[bestIdx] = true
			t.tracks[bestIdx].lastBox = det.GetBbox()
			t.tracks[bestIdx].missed = 0
			det.TrackId = t.tracks[bestIdx].id
			env.Tracks = append(env.Tracks, dataplane.TrackHit{DetectionIndex: di, TrackID: t.tracks[bestIdx].id})
		} else {
			ts := &trackState{id: newTrackID(), classLabel: det.GetClassLabel(), lastBox: det.GetBbox()}
			t.tracks = append(t.tracks, ts)
			used = append(used, true)
			det.TrackId = ts.id
			env.Tracks = append(env.Tracks, dataplane.TrackHit{DetectionIndex: di, TrackID: ts.id})
		}
	}
	// Age out tracks that didn't get matched this frame.
	survivors := t.tracks[:0]
	for ti, tr := range t.tracks {
		if !used[ti] {
			tr.missed++
		}
		if tr.missed <= t.cfg.MaxMissing {
			survivors = append(survivors, tr)
		}
	}
	t.tracks = survivors
}

// iou returns the intersection-over-union of two normalized bounding boxes.
func iou(a, b *dataplanev1.BoundingBox) float32 {
	if a == nil || b == nil {
		return 0
	}
	ax1, ay1, ax2, ay2 := a.GetX(), a.GetY(), a.GetX()+a.GetW(), a.GetY()+a.GetH()
	bx1, by1, bx2, by2 := b.GetX(), b.GetY(), b.GetX()+b.GetW(), b.GetY()+b.GetH()
	ix1 := maxF(ax1, bx1)
	iy1 := maxF(ay1, by1)
	ix2 := minF(ax2, bx2)
	iy2 := minF(ay2, by2)
	iw := ix2 - ix1
	ih := iy2 - iy1
	if iw <= 0 || ih <= 0 {
		return 0
	}
	inter := iw * ih
	areaA := (ax2 - ax1) * (ay2 - ay1)
	areaB := (bx2 - bx1) * (by2 - by1)
	union := areaA + areaB - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func newTrackID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "trk-" + hex.EncodeToString(b[:])
}

func maxF(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
func minF(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
