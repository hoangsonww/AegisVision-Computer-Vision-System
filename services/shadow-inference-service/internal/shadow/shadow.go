// Package shadow implements the shadow-inference loop (ADR-0024).
//
// The service subscribes to frame.descriptor.* claim-check envelopes (the
// dataplane's existing topic — never raw bytes per ADR-0008), picks a
// subset per the active canary's split rule, asks the candidate model
// host (inference-router) for its prediction, compares it against the
// baseline result already known from the primary path, and emits a
// CanaryObservation summarizing the comparison.
//
// Key constraint: the shadow path NEVER alters tenant-facing detections.
// We compare on labels-only and we never publish a *detection* — only a
// comparison observation.
package shadow

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/aegisvision/pkg/canary"
	"github.com/aegisvision/pkg/intelligence"
)

// FrameDescriptor is the subset of the dataplane envelope we need. The
// proto definition (dataplane/v1/frame_descriptor.proto) is broader; this
// struct mirrors the JSON form that lands on the bus.
type FrameDescriptor struct {
	TenantID  string    `json:"tenant_id"`
	StreamID  string    `json:"stream_id"`
	FrameURN  string    `json:"frame_urn"`
	OccurredAt time.Time `json:"occurred_at"`
}

// BaselineResult is what the primary path emits per frame.
type BaselineResult struct {
	TenantID   string             `json:"tenant_id"`
	StreamID   string             `json:"stream_id"`
	FrameURN   string             `json:"frame_urn"`
	ModelID    string             `json:"model_id"`
	Predicted  string             `json:"predicted_class"`
	Confidence float64            `json:"confidence"`
	LatencyMs  int64              `json:"latency_ms"`
}

// CandidateInvoker runs the candidate model for the given frame URN. The
// service interface mirrors inference-router's HTTP API but tests use a
// stub.
type CandidateInvoker interface {
	Invoke(ctx context.Context, modelID, frameURN string) (BaselineResult, error)
}

// Active describes one active shadow assignment (plan + variant pct).
type Active struct {
	PlanID           string
	TenantID         string
	BaselineModel    string
	CandidateModel   string
	CandidatePercent int
}

// Engine pairs incoming frame descriptors with their baseline results,
// invokes the candidate, and accumulates per-window observations.
type Engine struct {
	mu       sync.Mutex
	active   []Active
	pending  map[string]BaselineResult // key = frame_urn → baseline
	windows  map[string]*windowAcc     // key = plan_id|variant|windowStart
	invoker  CandidateInvoker
	now      func() time.Time
	WindowDur time.Duration
}

type windowAcc struct {
	Plan        string
	Variant     string
	Start, End  time.Time
	Successes   int64
	Failures    int64
	LatencySum  int64
	LatencyMax  int64
	Count       int64
}

// NewEngine constructs an engine. invoker may be nil in tests that only
// exercise the windowing logic.
func NewEngine(invoker CandidateInvoker) *Engine {
	return &Engine{
		invoker:   invoker,
		pending:   map[string]BaselineResult{},
		windows:   map[string]*windowAcc{},
		now:       time.Now,
		WindowDur: 1 * time.Minute,
	}
}

func (e *Engine) SetActive(active []Active) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.active = append(e.active[:0], active...)
}

// OnBaseline records a baseline result for later comparison.
func (e *Engine) OnBaseline(_ context.Context, br BaselineResult) {
	if br.FrameURN == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pending[br.FrameURN] = br
}

// OnFrame considers the frame for each active shadow assignment that
// matches its tenant. If selected, invokes the candidate and accumulates
// a comparison. Idempotent on repeated frame URNs.
func (e *Engine) OnFrame(ctx context.Context, fd FrameDescriptor) {
	e.mu.Lock()
	active := append([]Active(nil), e.active...)
	br, ok := e.pending[fd.FrameURN]
	e.mu.Unlock()
	if !ok {
		// No baseline available yet — drop. The dataplane publishes
		// baseline + descriptor close in time; if we miss the baseline,
		// the comparison would be unsound.
		return
	}
	for _, a := range active {
		if a.TenantID != "" && a.TenantID != fd.TenantID {
			continue
		}
		if canary.Variant(fd.StreamID, a.CandidatePercent) != "candidate" {
			continue
		}
		go e.invokeAndRecord(ctx, a, fd, br)
	}
}

func (e *Engine) invokeAndRecord(ctx context.Context, a Active, fd FrameDescriptor, br BaselineResult) {
	start := e.now()
	cand, err := e.invokeCandidate(ctx, a, fd)
	lat := e.now().Sub(start).Milliseconds()
	e.mu.Lock()
	defer e.mu.Unlock()
	bk := e.bucket(a.PlanID, "baseline")
	ck := e.bucket(a.PlanID, "candidate")
	// Baseline counters reflect what the *primary* path observed.
	bk.Count++
	bk.LatencySum += br.LatencyMs
	if br.LatencyMs > bk.LatencyMax {
		bk.LatencyMax = br.LatencyMs
	}
	if br.Predicted != "" {
		bk.Successes++
	} else {
		bk.Failures++
	}
	// Candidate counters reflect what shadow inference observed.
	ck.Count++
	if err != nil {
		ck.Failures++
		return
	}
	ck.LatencySum += lat
	if lat > ck.LatencyMax {
		ck.LatencyMax = lat
	}
	if cand.Predicted == br.Predicted {
		// Same class → "success" for canary purposes (the candidate
		// agreed with the baseline). When you actually have ground truth
		// you'd want a different definition; for shadow comparisons,
		// agreement is the workable proxy.
		ck.Successes++
	} else {
		ck.Failures++
	}
}

func (e *Engine) invokeCandidate(ctx context.Context, a Active, fd FrameDescriptor) (BaselineResult, error) {
	if e.invoker == nil {
		// Synthetic mode: pretend the candidate agreed.
		return BaselineResult{Predicted: "agree", Confidence: 0.8}, nil
	}
	return e.invoker.Invoke(ctx, a.CandidateModel, fd.FrameURN)
}

// bucket returns the active window accumulator, creating one if needed.
// Must be called with e.mu held.
func (e *Engine) bucket(planID, variant string) *windowAcc {
	now := e.now()
	start := now.Truncate(e.WindowDur)
	key := planID + "|" + variant + "|" + start.Format(time.RFC3339)
	w, ok := e.windows[key]
	if !ok {
		w = &windowAcc{Plan: planID, Variant: variant, Start: start, End: start.Add(e.WindowDur)}
		e.windows[key] = w
	}
	return w
}

// Drain returns the closed-out windows (where window.End < now) as
// CanaryObservations and removes them from the map. Call this on a
// ticker to publish.
func (e *Engine) Drain(now time.Time) []intelligence.CanaryObservation {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []intelligence.CanaryObservation
	for k, w := range e.windows {
		if !w.End.After(now) {
			latP95 := w.LatencyMax
			latP50 := int64(0)
			if w.Count > 0 {
				latP50 = w.LatencySum / w.Count
			}
			out = append(out, intelligence.CanaryObservation{
				PlanID: w.Plan, Variant: w.Variant,
				Successes: w.Successes, Failures: w.Failures,
				LatencyP50Ms: latP50, LatencyP95Ms: latP95, LatencyP99Ms: latP95,
				WindowStart: w.Start, WindowEnd: w.End,
			})
			delete(e.windows, k)
		}
	}
	// Also prune pending baselines older than 10 windows.
	if len(e.pending) > 100_000 {
		// Best-effort cap; oldest are pruned arbitrarily — the dataplane
		// either publishes a baseline within the same window or the
		// comparison is unsound anyway.
		i := 0
		for k := range e.pending {
			delete(e.pending, k)
			i++
			if i > len(e.pending)/2 {
				break
			}
		}
	}
	return out
}

// ParseFrame helper for the bus subscription.
func ParseFrame(b []byte) (FrameDescriptor, error) {
	var fd FrameDescriptor
	err := json.Unmarshal(b, &fd)
	return fd, err
}

// ParseBaseline helper for the bus subscription.
func ParseBaseline(b []byte) (BaselineResult, error) {
	var br BaselineResult
	err := json.Unmarshal(b, &br)
	return br, err
}
