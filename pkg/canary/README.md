# pkg/canary

> **Wilson lower-bound proportion test + minimum sample floor + traffic
> split.** ADR-0023.

Used by `canary-controller`. Pure math; no I/O.

---

## API

```go
// Decide returns Promote / Hold / Rollback.
func Decide(state State, params Params) Decision

// State per-arm running counters.
type State struct {
    Baseline ArmCounters
    Candidate ArmCounters
}
type ArmCounters struct {
    Successes uint64
    Failures  uint64
}

// Params from the canary plan.
type Params struct {
    MinSamples           uint64    // floor before any decision
    WilsonAlpha          float64   // significance, e.g. 0.05
    PromotionMarginPP    float64   // percentage points, e.g. 0.5
    RollbackMarginPP     float64   // percentage points, e.g. 2.0
}
```

---

## Why Wilson, not naive proportion

Wilson handles small samples and edge cases (0% / 100% success) without
exploding. Naive z-test confidence intervals are unbounded near 0/1.

---

## Split

```go
func ShouldRouteToCandidate(streamID string, trafficPct int) bool
```

Stable hash on `(streamID, planID)` → deterministic per-stream split.
A stream stays on its arm for the duration of the plan.

---

## See also

- ADR-0023.
- [`services/canary-controller/README.md`](../../services/canary-controller/README.md).
