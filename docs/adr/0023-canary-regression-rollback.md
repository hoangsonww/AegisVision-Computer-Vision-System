# ADR-0023: Canary statistical regression + auto-rollback policy

**Status:** Accepted (2026-05-21)

## Context

The canary controller promotes models and pipelines progressively:
5% → 25% → 50% → 100%. Between steps the controller must decide *is
the candidate worse than the baseline?* with confidence. The naive
choices are bad:

- **Equal-rate threshold (`candidate < baseline - 0.01`).** Triggers on
  noise at low traffic, ignores real regressions when the baseline is
  already saturated at 100%.
- **Naive t-test on every observation.** Multiple-comparisons problem;
  false positives compound across steps.
- **Manual promotion only.** Adaptive autonomy's whole point is
  removing this manual burden where it's safe to do so.

We need a test that handles small n gracefully and that is cheap to
recompute on every controller tick.

## Decision

Use a one-sided proportion test based on the **lower bound of the
Wilson score interval** for the candidate, gated by a hard
minimum-sample-size floor:

> "Healthy" when `baseline_rate − wilson_lower_95(candidate_successes,
> candidate_n) ≤ MaxProportionDelta`.

A second check fails the verdict if `candidate.p95_latency_ms −
baseline.p95_latency_ms > MaxLatencyP95Ms`.

If either fails: **immediate rollback** to step 0. Rollback is
reversible (the controller can restart the plan); promotion is the
*consequential* direction and goes through `policy-gate-service`.

Wilson lower bound was chosen because:
1. Well-defined for small n (where normal approximation collapses).
2. Anchored at 0.5 toward 0 (== conservative under small samples — the
   controller won't promote on weak evidence).
3. Single-pass arithmetic — fits inside the per-tick reconciliation
   without a dedicated stats library.

## Consequences

- **Promotion is gated; rollback is automatic.** Reflects the
  bounded-autonomy default: irreversible direction needs a human,
  reversible direction does not.
- **Latency-as-a-failure is on by default.** Even if the candidate
  matches accuracy, a 50% p95 latency regression rolls back.
- **No P-value soup.** We avoid multiple-comparisons issues by always
  computing the same single-shot Wilson test against the latest window.
- **Operators can pause + resume.** Pause is a reversible operator
  override; resume restarts the loop at the current step.

## Rejected alternatives

- **Sequential probability ratio test (SPRT).** Rejected — more
  powerful in theory, but harder to audit and tune; Wilson is enough
  for the 0.5–5pp deltas we care about.
- **Auto-promote at 100% by default.** Rejected — promotion is the
  irreversible direction; it must be a deliberate human act.
- **No latency check.** Rejected — accuracy-equivalent but slower
  models cause downstream SLO burn even with no detection regression.
