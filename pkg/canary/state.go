// Canary state machine. The controller's main loop calls Next() on each
// pending plan; Next() decides the next status + reason based on the
// current step, hold time, observed verdict, and the plan's ladder.
package canary

import (
	"time"

	"github.com/aegisvision/pkg/intelligence"
)

// Decision is the next action the controller should apply.
type Decision struct {
	NewStatus       intelligence.CanaryStatus
	NewStep         int     // -1 if unchanged
	FailureReason   string  // when transitioning to ROLLED_BACK / FAILED
	GateRequired    bool    // true means the controller must open a gate
	HumanReadable   string  // for audit
}

// Next produces a Decision given the current plan, the latest verdict
// from AssessRegression, and the current time.
//
//   - Holding step with a healthy verdict + hold elapsed → advance.
//   - Holding step with a regression verdict → rollback (or pause for
//     manual review if the regression is catastrophic).
//   - At the last step with auto_promote=false → HOLDING forever (manual
//     promotion).
//   - At the last step with auto_promote=true → PROMOTED, but only via
//     a gate (consequential — ADR-0023).
func Next(plan intelligence.CanaryPlan, verdict Verdict, now time.Time) Decision {
	if plan.Status == intelligence.CanaryStatusPromoted ||
		plan.Status == intelligence.CanaryStatusRolledBack ||
		plan.Status == intelligence.CanaryStatusFailed ||
		plan.Status == intelligence.CanaryStatusPaused {
		return Decision{NewStatus: plan.Status, NewStep: -1, HumanReadable: "terminal"}
	}
	if len(plan.Ladder) == 0 {
		return Decision{NewStatus: intelligence.CanaryStatusFailed, NewStep: -1,
			FailureReason: "empty_ladder", HumanReadable: "empty ladder"}
	}
	step := plan.CurrentStep
	if step < 0 {
		step = 0
	}
	if step >= len(plan.Ladder) {
		// At the end. Decide based on auto_promote.
		if plan.AutoPromote {
			return Decision{NewStatus: intelligence.CanaryStatusPromoted, NewStep: step,
				GateRequired: true, HumanReadable: "auto_promote at end of ladder (gated)"}
		}
		return Decision{NewStatus: intelligence.CanaryStatusHolding, NewStep: step,
			HumanReadable: "holding at last step; awaiting manual promote"}
	}
	current := plan.Ladder[step]
	// Regression is a hard signal regardless of hold time.
	if !verdict.Healthy && verdict.Reason != "insufficient_samples" {
		return Decision{
			NewStatus: intelligence.CanaryStatusRolledBack, NewStep: 0,
			FailureReason: verdict.Reason,
			HumanReadable: "regression detected: " + verdict.Reason,
		}
	}
	// Below the sample floor: keep observing.
	if verdict.Reason == "insufficient_samples" {
		return Decision{NewStatus: intelligence.CanaryStatusHolding, NewStep: step,
			HumanReadable: "awaiting more samples at step"}
	}
	// Healthy + hold elapsed → advance.
	if !plan.LastStepAt.IsZero() && now.Sub(plan.LastStepAt).Seconds() >= float64(current.HoldSeconds) {
		next := step + 1
		if next >= len(plan.Ladder) {
			if plan.AutoPromote {
				return Decision{NewStatus: intelligence.CanaryStatusPromoted, NewStep: next,
					GateRequired: true, HumanReadable: "advancing to PROMOTED (gated)"}
			}
			return Decision{NewStatus: intelligence.CanaryStatusHolding, NewStep: next,
				HumanReadable: "advancing to last step; holding for manual promote"}
		}
		return Decision{NewStatus: intelligence.CanaryStatusRamping, NewStep: next,
			HumanReadable: "advancing to next step"}
	}
	return Decision{NewStatus: intelligence.CanaryStatusHolding, NewStep: step,
		HumanReadable: "holding until window elapses"}
}
