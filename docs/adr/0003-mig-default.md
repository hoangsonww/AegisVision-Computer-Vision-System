# ADR-0003: MIG is the default GPU sharing mode for production inference

- Status: Accepted
- Date: 2026-05-17

## Context

A single GPU hosts multiple inference workloads concurrently. Three sharing
modes exist:

- **MIG** (Multi-Instance GPU): hard partition of memory + compute units.
- **MPS** (Multi-Process Service): soft co-residency; kernels interleave on
  the SM scheduler.
- **Time-slicing**: scheduler round-robins; no isolation.

In production we observed that a single mis-estimated model
(VRAM under-counted, or an unexpected input shape) drops inference on every
co-located neighbor. The blast radius of an estimation error is the entire
GPU.

## Decision

MIG is the **default** for production inference. MPS is permitted *only*
inside a single tenant's trusted set of co-located models. Time-slicing is
permitted only in dev.

## Consequences

- Per-model cost profiles (VRAM, ms) are mandatory: a model cannot be
  promoted without a measured profile that fits a MIG slice.
- Bin-packing across MIG slices is the GPU scheduler's primary job.
- Some VRAM headroom is unrecoverable; we treat it as the cost of
  isolation, not waste.
- KAI Scheduler is selected for fractional packing precisely because its
  model assumes MIG-class instances.
