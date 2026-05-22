// Package loop is the canary-controller's main reconciliation loop.
//
// On each tick it pulls every active plan, computes the candidate vs
// baseline verdict over the recent observation window, asks the state
// machine for the next decision, and applies it. Promotion (gated) and
// rollback (immediate) are emitted as bus events so model-registry can
// shift traffic + audit-service can record.
package loop

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/aegisvision/pkg/bus"
	"github.com/aegisvision/pkg/canary"
	"github.com/aegisvision/pkg/intelligence"

	"github.com/aegisvision/services/canary-controller/internal/store"
)

// Reconciler drives the state machine. Construct one per service
// instance; safe for concurrent ticks.
type Reconciler struct {
	Store   *store.Store
	Bus     bus.Publisher
	Logger  *slog.Logger
	Lookback time.Duration // window over which to compute verdicts; default 5m
}

func (r *Reconciler) lookback() time.Duration {
	if r.Lookback > 0 {
		return r.Lookback
	}
	return 5 * time.Minute
}

// Tick reconciles every active plan once. Returns the number of plans
// that produced a state change.
func (r *Reconciler) Tick(ctx context.Context, now time.Time) (int, error) {
	plans, err := r.Store.ListActive(ctx)
	if err != nil {
		return 0, err
	}
	changes := 0
	for _, p := range plans {
		// Skip draft + paused — they're operator-controlled.
		if p.Status == intelligence.CanaryStatusDraft || p.Status == intelligence.CanaryStatusPaused {
			continue
		}
		if changed, err := r.tickOne(ctx, p, now); err != nil {
			r.log("tick failed", slog.String("plan", p.ID), slog.String("err", err.Error()))
		} else if changed {
			changes++
		}
	}
	return changes, nil
}

func (r *Reconciler) tickOne(ctx context.Context, p intelligence.CanaryPlan, now time.Time) (bool, error) {
	if len(p.Ladder) == 0 || p.CurrentStep >= len(p.Ladder) {
		return false, nil
	}
	step := p.Ladder[p.CurrentStep]
	since := now.Add(-r.lookback())
	baseline, err := r.Store.AggregateSince(ctx, p.ID, "baseline", since)
	if err != nil {
		return false, err
	}
	candidate, err := r.Store.AggregateSince(ctx, p.ID, "candidate", since)
	if err != nil {
		return false, err
	}
	verdict := canary.AssessRegression(canary.RegressionInput{
		Baseline:           baseline,
		Candidate:          candidate,
		MaxProportionDelta: p.MaxProportionDelta,
		MaxLatencyP95Ms:    p.MaxLatencyP95MsDelta,
		MinSamples:         step.MinSamples,
	})
	d := canary.Next(p, verdict, now)
	if d.NewStatus == p.Status && d.NewStep == p.CurrentStep {
		return false, nil
	}
	if err := r.Store.ApplyDecision(ctx, p.ID, d); err != nil {
		return false, err
	}
	r.emitTransition(ctx, p, d)
	return true, nil
}

func (r *Reconciler) emitTransition(ctx context.Context, p intelligence.CanaryPlan, d canary.Decision) {
	body, _ := json.Marshal(map[string]any{
		"plan_id": p.ID, "tenant_id": p.TenantID,
		"resource_urn": p.ResourceURN,
		"from": string(p.Status), "to": string(d.NewStatus),
		"step": d.NewStep, "reason": d.HumanReadable, "failure": d.FailureReason,
	})
	subj := "canary.transition.v1"
	switch d.NewStatus {
	case intelligence.CanaryStatusRolledBack:
		subj = "canary.rollback.v1"
	case intelligence.CanaryStatusPromoted:
		subj = "canary.promote.v1"
	}
	_ = r.Bus.Publish(ctx, bus.Message{
		Subject: subj, Data: body,
		Headers: map[string]string{"tenant_id": p.TenantID, "plan_id": p.ID},
	})
	r.log("transition",
		slog.String("plan", p.ID), slog.String("to", string(d.NewStatus)),
		slog.String("reason", d.HumanReadable))
}

func (r *Reconciler) log(msg string, args ...any) {
	if r.Logger == nil {
		return
	}
	r.Logger.Info(msg, args...)
}
