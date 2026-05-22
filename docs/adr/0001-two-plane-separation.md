# ADR-0001: Hard separation of control plane and data plane

- Status: Accepted
- Date: 2026-05-17
- Deciders: Architecture Working Group

## Context

A user pipeline ("when a person enters the dock zone, emit a HIGH-severity
event") has two very different lifetimes living inside it:

- a **lifecycle** that changes seconds-to-days at a time — created, validated,
  deployed, paused, retired — which needs durable orchestration with
  retries, signals, timers, history;
- a **per-frame execution path** running at 30 fps × 10,000 streams =
  300,000 events/sec — which cannot pay the cost of a durable workflow
  engine per step.

Designing the system around either of those would compromise the other.

## Decision

Split the system into two planes, treated as separate engineering domains:

- **Control plane**: Temporal-driven orchestration. State in PostgreSQL.
  Owns pipeline / model / training / deployment lifecycles. Never sees a
  video frame.
- **Data plane**: streaming operators connected by bounded queues at line
  rate. State ephemeral; checkpoints in Redis; the firehose lands in
  ClickHouse. Never persists per-step history.

A user-authored pipeline **compiles into both** — a Temporal workflow that
manages the lifecycle, AND a data-plane operator graph that processes
frames.

## Consequences

- **Required**: every new feature must be assigned to a plane on day one.
  "Where does this state live?" and "what is its hot-path rate?" are
  rephrased: control plane or data plane.
- A bug that "puts the frame loop in Temporal" or "persists a detection
  per Temporal activity" is not a small bug — it crosses an
  architectural boundary and must be rejected at review.
- New operators on the data plane must not call control-plane APIs in
  the hot path. They subscribe to materialized config (NATS / config
  cache); the gateway lookup is for cold starts only.
