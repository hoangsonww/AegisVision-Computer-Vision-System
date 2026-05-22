# ADR-0025: Drift detection + SLO signals

**Status:** Accepted (2026-05-21)

## Context

Phase 5 autonomy agents need to know *what changed*. Two signal classes
matter:

- **Concept drift** — the distribution of inputs the model sees today
  is different from the distribution it was promoted on. Drift doesn't
  fail any SLO directly; the model's accuracy may still be high on
  whatever's actually arriving. But it predicts that accuracy is going
  to fall, and it tells the monitor agent to investigate now.
- **SLO burn** — direct evidence of user-visible degradation. Burn-rate
  alerting per the Google SRE workbook table 5-7 gives us a
  multi-window severity classification.

Both signals must be tenant-scoped and operationally cheap.

## Decision

### Drift

`drift-detection-service` keeps a rolling per-(tenant, model) class-count
window (configurable; default 15 minutes), normalizes it to a
distribution, and computes divergence against a stored reference
distribution captured at model-promotion time.

Three divergence measures supported:

- **KL** — sensitive but unbounded; useful as a fast tripwire.
- **JS** — symmetric, bounded [0, 1] in base-2; default.
- **TVD** — interpretable as the maximum probability mass that moved;
  useful for thresholds operators understand intuitively.

Drift signals are emitted as `autonomy.signal.drift.v1` with severity:
`info` (informational), `warn` (>= warn_threshold), `critical` (>=
critical_threshold). Each tenant-model has its own policy with its own
thresholds — the same JS divergence may be alarming for a
narrow-class-set model and routine for a broad-class-set one.

### SLO

`slo-watchdog` runs every minute, queries Prometheus for each active
`SLOTarget` over 1h/6h/24h windows, computes the burn rate from
`(1 - observed) / (1 - target)`, and applies the multi-window rule:

| 1h burn | 6h burn | 24h burn | Severity  |
| ------- | ------- | -------- | --------- |
| ≥ 14    | ≥ 6     | —        | critical  |
| ≥ 6     | ≥ 3     | —        | warn      |
| —       | —       | ≥ 1      | info      |

Each crossing emits `autonomy.signal.slo.v1`.

## Consequences

- **Operators tune thresholds.** Defaults are conservative; tenants
  with high-variance class distributions will set warn=0.2 / critical=0.4
  instead of 0.1 / 0.3.
- **Reference distributions must be re-captured at every model
  promotion.** Otherwise drift compares against an ancient baseline
  and false-fires constantly. Recapture is a step in the canary
  promotion gate runbook.
- **No frames or PII in drift signals.** Drift signals carry class
  *counts* and *names* — no payload. PII can still hide in class
  names (e.g. license-plate text), so the redaction operator
  (Phase 3) is upstream of the class label.
- **Multi-window burn rates are sticky.** A single bad minute won't
  fire critical; that's intentional — paging on noise is the most
  expensive SRE failure mode.

## Rejected alternatives

- **PSI (Population Stability Index).** Rejected for the default —
  bucketed continuous distributions, mismatch with discrete class
  vocabularies.
- **Naive error-budget gauge with no burn-rate math.** Rejected —
  doesn't distinguish "burning fast over 5 minutes" from "drifting
  slowly over 14 days." The multi-window rule is the standard for a
  reason.
