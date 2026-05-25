# ADR-0024: Shadow inference architecture

**Status:** Accepted (2026-05-21)

## Context

The canary controller (ADR-0023) needs *both* a baseline and a
candidate result per frame to compute a comparison. The naive approach
is to actually serve the candidate to a percentage of tenant traffic —
but that *is* the canary itself, and it means the candidate's behavior
already affects the tenant.

We want a path that lets us evaluate a candidate *without* exposing the
tenant to it at all — a "shadow" inference path.

## Decision

`shadow-inference-service`:

1. Subscribes to `frame.descriptor.*` (claim-check URN only —
   ADR-0008) and `inference.baseline.v1` (the primary path's emitted
   result).
2. When both arrive for the same frame URN, applies the canary's
   deterministic traffic split (`pkg/canary.Variant`) to select a
   subset for shadowing.
3. For selected frames, asks `inference-router` to run the candidate
   model on the same frame URN — bytes are never duplicated; the
   inference engine dereferences the claim-check.
4. Compares the candidate's predicted class against the baseline's
   (label agreement is the workable proxy when no ground truth is
   available) and accumulates per-window CanaryObservations.
5. Flushes closed windows to `canary-controller.observations`.

The service NEVER publishes a tenant-facing detection. Its only
outputs are observations + metrics.

## Consequences

- **Tenant traffic is unaffected.** Shadow inference is purely an
  observer. A buggy candidate cannot leak into operator alerts via
  this path.
- **GPU cost grows.** Shadowing N% of traffic costs N% of the
  candidate model's inference cost. The split percentage in
  `CanaryPlan.ladder` controls this directly.
- **No raw frame transfer.** Both the primary and the shadow path
  reference the same URN; the dataplane's claim-check ring is the
  single retrieval point.
- **Label agreement ≠ accuracy.** When the baseline is wrong, the
  candidate "disagrees correctly." For real promotions, the operator
  pairs shadow with active-learning labeled samples to validate
  agreement-vs-ground-truth before approving.

## Rejected alternatives

- **Live A/B in production.** Rejected for early steps in the ladder
  — exposes tenant to the candidate's failures.
- **Duplicate the frame to a "candidate" Kafka topic.** Rejected per
  ADR-0008 — frames don't travel the bus.
- **Run the candidate as a sidecar of every dataplane pod.** Rejected
  — couples deploy lifecycles and breaks tenant isolation.
