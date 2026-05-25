# ADR-0026: Predictive cache prefetch

**Status:** Accepted (2026-05-21)

## Context

Cold-start latency for production inference is dominated by:

1. Triton loading the model weights (1–10s for typical CV models).
2. The inference-router's per-tenant CUDA context warming.

The naive answer — keep everything always loaded — costs VRAM the
platform doesn't have. The other naive answer — load on demand — adds a
multi-second latency spike to the first request of a tenant after an
idle window.

We want to load the right models *just before* they're needed, without
keeping them all in VRAM all the time.

## Decision

`prefetch-service`:

1. Subscribes to `detections.v1` and bumps a per-(tenant, model)
   counter into a 7×24 grid of (day-of-week, hour) cells.
2. Smooths each cell with an EMA (default alpha=0.2 → 5-week
   half-life).
3. On a 60-second tick, predicts the top-N (tenant, model) pairs for
   the horizon-ahead cell (default +5 min) and sends idempotent
   warm-up RPCs to inference-router.

The predictor is intentionally simple:

- 7×24 = 168 cells per (tenant, model) pair.
- EMA gives stable predictions without overreacting to single days.
- Top-N cap (default 16) keeps the inference-router from being
  overloaded by warm-ups.

## Consequences

- **First-request latency falls without VRAM bloat.** Models that the
  tenant will use in 5 minutes are already loaded; models the tenant
  rarely uses at this hour aren't.
- **No personally-identifiable data in the predictor.** Only
  (tenant, model) pairs and counts.
- **No predictive prefetch on the edge.** Edge sites have fixed
  per-camera model assignments — the grid wouldn't help. The
  edge-profile chart disables this service.
- **Privacy-by-construction.** The predictor doesn't see frame data,
  only the (tenant, model) tuple from detection events. Upstream PII
  redaction is preserved.

## Rejected alternatives

- **Pin every model always.** Rejected — VRAM cost scales with model
  count × tenants.
- **Per-tenant ARMA / Prophet model.** Rejected — overkill given the
  load pattern; revisit if EMA stops being accurate enough.
- **LSTM-based predictor.** Rejected — too much operational cost
  (training, drift, retraining) for a problem with a clear floor
  built into the time-of-week structure.
