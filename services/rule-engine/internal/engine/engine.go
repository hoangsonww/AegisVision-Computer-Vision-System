// Package engine is the CEP-style rule evaluator for events. The dataplane
// emits raw detections; the rule engine emits HIGH-severity events when
// composable conditions match (zone dwell, line cross, abandoned object,
// rate, AND/OR composition).
//
// The engine is intentionally small and pure-Go: stateless predicates plus a
// per-(track,rule) state map. State entries expire after a configurable
// TTL so memory stays bounded under camera churn.
package engine

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	commonv1 "github.com/aegisvision/proto/gen/go/aegisvision/common/v1"
	dataplanev1 "github.com/aegisvision/proto/gen/go/aegisvision/dataplane/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RuleKind enumerates supported rule shapes.
type RuleKind string

const (
	KindDwell         RuleKind = "spatial.dwell"
	KindLineCross     RuleKind = "spatial.line_cross"
	KindAbandoned     RuleKind = "temporal.abandoned"
	KindRate          RuleKind = "temporal.rate"
	KindComposite     RuleKind = "composite"
)

// Zone is a normalized rectangle.
type Zone struct {
	ID         string
	X, Y, W, H float32
}

// Line is two normalized endpoints; "cross" is detected by the sign of the
// 2D cross-product against the line direction flipping.
type Line struct {
	ID                 string
	X1, Y1, X2, Y2     float32
	// Direction defines positive cross direction; events fire only when a
	// track crosses in that direction. "any" disables direction filtering.
	Direction string
}

// Rule is one configured rule.
type Rule struct {
	ID         string
	TenantID   string
	PipelineID string
	Kind       RuleKind
	Severity   commonv1.Severity

	// Spatial / temporal parameters.
	Zone     *Zone
	Line     *Line
	Dwell    time.Duration
	Stale    time.Duration // for KindAbandoned: how long static = abandoned
	RateN    int           // for KindRate: count threshold
	RateWin  time.Duration // for KindRate: window

	// ComposedOf is the AND/OR composition for KindComposite. The engine
	// evaluates each sub-rule independently; firing semantics:
	//   - "and": fire when ALL sub-rules are currently firing
	//   - "or":  fire when ANY sub-rule fires (deduplicated per cooldown)
	ComposedOf []*Rule
	Composer   string // "and" | "or"
	Cooldown   time.Duration
}

type state struct {
	enteredAt time.Time
	fired     bool
	lastFired time.Time
	// For line-cross: the previous-frame side of the line.
	prevSide float32
	// For abandoned: pose stability window start.
	stableSince time.Time
	stableBBox  *dataplanev1.BoundingBox
	// For rate: timestamps of recent triggers within RateWin.
	hits []time.Time
}

// Engine evaluates rules against streaming detections.
type Engine struct {
	mu     sync.Mutex
	rules  []*Rule
	states map[string]*state // key = ruleID + ":" + trackID
	StateTTL time.Duration
	now    func() time.Time
}

// New constructs an Engine.
func New(rules []*Rule) *Engine {
	return &Engine{
		rules:    rules,
		states:   map[string]*state{},
		StateTTL: 10 * time.Minute,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Apply evaluates every rule against one detection. Returns any events that
// fired this tick.
func (e *Engine) Apply(d *dataplanev1.Detection) []*dataplanev1.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []*dataplanev1.Event
	now := captureOrNow(d, e.now)
	for _, r := range e.rules {
		if ev := e.evalRule(r, d, now); ev != nil {
			out = append(out, ev)
		}
	}
	e.gcLocked(now)
	return out
}

func (e *Engine) evalRule(r *Rule, d *dataplanev1.Detection, now time.Time) *dataplanev1.Event {
	switch r.Kind {
	case KindDwell:
		return e.evalDwell(r, d, now)
	case KindLineCross:
		return e.evalLineCross(r, d, now)
	case KindAbandoned:
		return e.evalAbandoned(r, d, now)
	case KindRate:
		return e.evalRate(r, d, now)
	case KindComposite:
		return e.evalComposite(r, d, now)
	}
	return nil
}

func (e *Engine) evalDwell(r *Rule, d *dataplanev1.Detection, now time.Time) *dataplanev1.Event {
	if r.Zone == nil || d.GetTrackId() == "" {
		return nil
	}
	key := r.ID + ":" + d.GetTrackId()
	if !centerInZone(*r.Zone, d.GetBbox()) {
		delete(e.states, key)
		return nil
	}
	s, ok := e.states[key]
	if !ok {
		e.states[key] = &state{enteredAt: now}
		return nil
	}
	if cooldownActive(s, r, now) {
		return nil
	}
	if now.Sub(s.enteredAt) >= r.Dwell {
		s.fired = true
		s.lastFired = now
		return buildEvent(r, d, now, "dwell rule fired")
	}
	return nil
}

func (e *Engine) evalLineCross(r *Rule, d *dataplanev1.Detection, now time.Time) *dataplanev1.Event {
	if r.Line == nil || d.GetTrackId() == "" {
		return nil
	}
	bb := d.GetBbox()
	if bb == nil {
		return nil
	}
	side := crossSide(*r.Line, bb.GetX()+bb.GetW()/2, bb.GetY()+bb.GetH()/2)
	key := r.ID + ":" + d.GetTrackId()
	s, ok := e.states[key]
	if !ok {
		e.states[key] = &state{prevSide: side, enteredAt: now}
		return nil
	}
	if signFlipped(s.prevSide, side) && !cooldownActive(s, r, now) {
		if r.Line.Direction != "" && r.Line.Direction != "any" {
			// Direction-filtered crossing: positive => prevSide < 0 && side > 0.
			if r.Line.Direction == "positive" && !(s.prevSide < 0 && side > 0) {
				s.prevSide = side
				return nil
			}
			if r.Line.Direction == "negative" && !(s.prevSide > 0 && side < 0) {
				s.prevSide = side
				return nil
			}
		}
		s.lastFired = now
		s.prevSide = side
		return buildEvent(r, d, now, "line crossed")
	}
	s.prevSide = side
	return nil
}

func (e *Engine) evalAbandoned(r *Rule, d *dataplanev1.Detection, now time.Time) *dataplanev1.Event {
	// Abandoned-object: bounding box stays within a small displacement
	// budget for r.Stale duration. Suitable for unattended-luggage / pallet
	// detectors.
	if r.Stale == 0 || d.GetTrackId() == "" {
		return nil
	}
	key := r.ID + ":" + d.GetTrackId()
	s, ok := e.states[key]
	if !ok {
		e.states[key] = &state{stableSince: now, stableBBox: d.GetBbox()}
		return nil
	}
	if !nearby(s.stableBBox, d.GetBbox(), 0.02) {
		s.stableSince = now
		s.stableBBox = d.GetBbox()
		return nil
	}
	if cooldownActive(s, r, now) {
		return nil
	}
	if now.Sub(s.stableSince) >= r.Stale {
		s.lastFired = now
		return buildEvent(r, d, now, "abandoned object")
	}
	return nil
}

func (e *Engine) evalRate(r *Rule, d *dataplanev1.Detection, now time.Time) *dataplanev1.Event {
	if r.RateN <= 0 || r.RateWin == 0 {
		return nil
	}
	// Per-stream rate: count of detections within RateWin.
	key := r.ID + ":" + d.GetStreamId()
	s, ok := e.states[key]
	if !ok {
		s = &state{}
		e.states[key] = s
	}
	cutoff := now.Add(-r.RateWin)
	filtered := s.hits[:0]
	for _, t := range s.hits {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	filtered = append(filtered, now)
	s.hits = filtered
	if len(s.hits) >= r.RateN && !cooldownActive(s, r, now) {
		s.lastFired = now
		return buildEvent(r, d, now, "detection rate exceeded")
	}
	return nil
}

// evalComposite evaluates the composed sub-rules. Sub-rules share the
// engine's state map keyed by their own IDs.
func (e *Engine) evalComposite(r *Rule, d *dataplanev1.Detection, now time.Time) *dataplanev1.Event {
	fires := 0
	for _, sub := range r.ComposedOf {
		if e.evalRule(sub, d, now) != nil {
			fires++
		}
	}
	want := len(r.ComposedOf)
	if r.Composer == "or" {
		want = 1
	}
	if fires >= want && want > 0 {
		key := r.ID + ":" + d.GetStreamId()
		s, ok := e.states[key]
		if !ok {
			s = &state{}
			e.states[key] = s
		}
		if cooldownActive(s, r, now) {
			return nil
		}
		s.lastFired = now
		return buildEvent(r, d, now, "composite rule fired")
	}
	return nil
}

// Cleanup expired keys so memory stays bounded.
func (e *Engine) gcLocked(now time.Time) {
	if e.StateTTL == 0 {
		return
	}
	for k, s := range e.states {
		ref := s.lastFired
		if ref.IsZero() {
			ref = s.enteredAt
		}
		if ref.IsZero() {
			ref = s.stableSince
		}
		if !ref.IsZero() && now.Sub(ref) > e.StateTTL {
			delete(e.states, k)
		}
	}
}

func centerInZone(z Zone, b *dataplanev1.BoundingBox) bool {
	if b == nil {
		return false
	}
	cx, cy := b.GetX()+b.GetW()/2, b.GetY()+b.GetH()/2
	return cx >= z.X && cx <= z.X+z.W && cy >= z.Y && cy <= z.Y+z.H
}

// crossSide returns the signed cross-product of (line.end - line.start) and
// (point - line.start). Sign indicates which side of the line the point is on.
func crossSide(l Line, x, y float32) float32 {
	dx := l.X2 - l.X1
	dy := l.Y2 - l.Y1
	return dx*(y-l.Y1) - dy*(x-l.X1)
}

func signFlipped(a, b float32) bool { return a*b < 0 }

func nearby(a, b *dataplanev1.BoundingBox, tol float32) bool {
	if a == nil || b == nil {
		return false
	}
	abs := func(f float32) float32 {
		if f < 0 {
			return -f
		}
		return f
	}
	return abs(a.GetX()-b.GetX()) < tol && abs(a.GetY()-b.GetY()) < tol &&
		abs(a.GetW()-b.GetW()) < tol && abs(a.GetH()-b.GetH()) < tol
}

func cooldownActive(s *state, r *Rule, now time.Time) bool {
	if r.Cooldown == 0 {
		return false
	}
	return !s.lastFired.IsZero() && now.Sub(s.lastFired) < r.Cooldown
}

func buildEvent(r *Rule, d *dataplanev1.Detection, now time.Time, summary string) *dataplanev1.Event {
	zoneID := ""
	if r.Zone != nil {
		zoneID = r.Zone.ID
	} else if r.Line != nil {
		zoneID = r.Line.ID
	}
	return &dataplanev1.Event{
		Id:          newEventID(),
		TenantId:    r.TenantID,
		PipelineId:  r.PipelineID,
		StreamId:    d.GetStreamId(),
		Kind:        dataplanev1.EventKind_EVENT_KIND_RULE_FIRED,
		Severity:    r.Severity,
		FrameSeq:    d.GetFrameSeq(),
		CaptureTime: d.GetCaptureTime(),
		EmittedAt:   timestamppb.New(now),
		Summary:     summary,
		RuleId:      r.ID,
		ZoneId:      zoneID,
		TrackId:     d.GetTrackId(),
		ClassLabel:  d.GetClassLabel(),
		Score:       d.GetScore(),
		BboxX:       d.GetBbox().GetX(),
		BboxY:       d.GetBbox().GetY(),
		BboxW:       d.GetBbox().GetW(),
		BboxH:       d.GetBbox().GetH(),
	}
}

func captureOrNow(d *dataplanev1.Detection, nowFn func() time.Time) time.Time {
	if t := d.GetCaptureTime(); t != nil && !t.AsTime().IsZero() {
		return t.AsTime()
	}
	return nowFn()
}

func newEventID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "evt-" + hex.EncodeToString(b[:])
}
