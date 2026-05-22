# ADR-0019: Active learning loop architecture

**Status:** Accepted (2026-05-21)

## Context

Production detection accuracy degrades over time as the world changes —
new uniforms, new vehicle models, new lighting conditions. The naive fix
is to retrain on the firehose. The operational safeguard (memory:
``) is that the firehose is poisoned by
trivial-confidence-true and trivial-confidence-false detections; training
on it amplifies existing biases.

We need to learn from the inputs the model is *uncertain* about, and feed
those to a human for ground-truth labels.

## Decision

The active learning loop is implemented as `active-learning-service`:

1. Subscribe to `detections.v1` from the firehose.
2. For each event, find the tenant's `LearningPolicy` for that model.
3. Pre-gate on `sample_rate` (typically 1–5%).
4. Compute the uncertainty score under the policy's strategy
   (least-confidence / margin / entropy / disagreement).
5. Accept only if the score is inside `[min_uncertainty, max_uncertainty]`.
6. Apply diversity bucketing — refuse samples that would exceed the
   per-class daily quota.
7. Apply per-tenant daily budget.
8. If still accepted: claim-check the frame snapshot, queue an annotation
   task for annotation-service, persist a `Sample` row.
9. When the operator labels it, mark `labeled`, publish
   `learning.labeled.v1` — training-orchestrator picks up the labeled
   samples for the next training run.

## Consequences

- **Frames never travel on the bus.** Only the URN does (ADR-0008). The
  snapshot is copy-on-promote per operational safeguard; the original frame is
  unaffected.
- **Per-tenant policy.** Tenants pick their strategy, sample rate,
  uncertainty band, diversity classes, and daily budget. Policy fits in
  one row.
- **Hard cap on human burden.** Daily budget is a hard cap; sampler
  refuses past it. Per-class soft cap prevents single-class collapse.
- **Auditable.** Every accepted/rejected decision is countable
  (`aegis_al_samples_queued_total` / `aegis_al_samples_rejected_total
  {reason}`).
- **TTL on snapshots.** Per-policy TTL bounds storage; expired un-labeled
  samples are pruned (sweep job, daily).

## Rejected alternatives

- **Random sampling.** Rejected — wastes the human's labels on
  not-uncertain examples that don't move the loss surface.
- **In-line during inference.** Rejected — that puts the sampler on the
  hot path. Bus-driven keeps detection latency unaffected.
- **Cross-tenant learning.** Rejected — labels are tenant property; we
  never pool them. (Federated learning is a separate, future ADR.)
