# Concepts

> **The vocabulary of AegisVision.** Read this once.

This document defines the resources, relationships, and lifecycle of
the platform — the *mental model* you need before any other doc makes
sense.

---

## Resources

```mermaid
erDiagram
    TENANT ||--o{ PROJECT : owns
    PROJECT ||--o{ PIPELINE : has
    PROJECT ||--o{ STREAM : has
    PROJECT ||--o{ MODEL : permits
    PROJECT ||--o{ DATASET : has
    PIPELINE ||--o{ PIPELINE_REVISION : versions
    STREAM }o--|| PIPELINE_REVISION : "runs against"
    STREAM ||--o{ EVENT : emits
    DATASET ||--o{ DATASET_VERSION : versions
    DATASET_VERSION ||--o{ SAMPLE : "contains"
    SAMPLE }o--|| LABEL_POLICY_REVISION : labeled-with
    MODEL ||--o{ MODEL_VERSION : versions
    MODEL_VERSION }o--|| DATASET_VERSION : "trained on"
```

### Tenant

The top-level isolation boundary. Every other resource belongs to
exactly one tenant. Tenants have a Vault transit key (crypto-shredding,
ADR-0014), a model allow-list, a plan, and a retention policy.

### Project

Grouping inside a tenant. Lets a tenant separate environments (prod /
staging), business lines, or teams. Members have project-scoped roles.

### Pipeline

A *recipe*. An ordered DAG of operators: `ingest → sampler → detect →
tracker → rule → emit`. Pipelines are **immutable per revision**.
Editing produces a new `PipelineRevision`. Streams reference a specific
revision; promoting a new revision is a deliberate, audited action.

### Stream

A producer of frames. Examples:

- `rtsp://camera-7/stream`
- `file:///recordings/2026-05-21/dock-1.mkv`
- `webrtc://browser-upload/xxx`
- `synthetic://noop` (dev / tests)

A stream picks a project, a pipeline revision, and a model. The data
plane runs the pipeline against the stream until paused or deleted.

### Model

A computer-vision model identity. Has **versions**; each version is an
immutable artifact (object-store URL + cosign signature). Models have
a **reference distribution** (for drift detection) and a **canary
plan** (for promotion).

### Dataset

A named collection of samples (frames + labels). Datasets have
**versions**. Lineage records which dataset version trained which
model version.

### Sample

One labeled frame. Carries a `frame_urn` (claim-check), a list of
labels (via `annotation-service`), and source provenance (which
stream, when).

### Label Policy

The schema of what classes / attributes exist. **Immutable per
revision** — a model trained against "car" continues to refer to
"car" even after a future revision renames it to "vehicle."

### Event

The output of the data plane. An event is *the thing the platform
tells you about*. Kinds: `DETECTION`, `RULE`, `DWELL`, `COUNT`,
`INFER`. Persisted to ClickHouse + fanned out via SSE / WebSocket.

### Detection

A model output (bounding box + class + confidence) on a single frame.
Detections become events when a rule trips. They are also fed
downstream to drift detection.

### Rule

A predicate over detections. `dwell(class=person, zone=A, min_ms=5000)`
fires an event when a person remains in zone A for ≥ 5s. Composable
with `AND` / `OR` / `NOT`. See [`services/rule-engine/README.md`](../services/rule-engine/README.md).

### Pipeline Revision

A specific version of a pipeline DAG. `Stream.pipeline_revision_id`
points at one. Promoting a new revision is an audited action.

### Canary Plan

A statistical comparison of two model versions against the same
streams. Wilson lower-bound proportion test + minimum sample floor +
margin. See [`services/canary-controller/README.md`](../services/canary-controller/README.md).

### Gate

A human-in-the-loop approval request. Required for tier-3 agent tools
and tier-3 actions (model promotion, retention override, force
failover). Routed through `policy-gate-service`.

### Audit Record

Append-only, hash-chained log entry. Every mutating action writes
one. See [`services/audit-service/README.md`](../services/audit-service/README.md).

---

## Lifecycle: a stream from creation to event

```mermaid
sequenceDiagram
    autonumber
    actor U as User
    participant AG as api-gateway
    participant SM as stream-manager
    participant DR as dataplane-runner
    participant IR as inference-router
    participant TRT as Triton
    participant ES as event-service
    participant CH as ClickHouse

    U->>AG: POST /v1/streams { pipeline_id, url }
    AG->>SM: gRPC CreateStream
    SM->>SM: validate, persist, assign shard k
    SM->>DR: operator.control.k (StartStream)
    DR->>DR: build operator DAG
    loop frame loop
        DR->>DR: ingest → sampler
        DR->>IR: gRPC Infer(frame_urn, model)
        IR->>TRT: detect
        TRT-->>IR: detections
        IR-->>DR: detections
        DR->>DR: tracker → rule
        alt rule trips
            DR->>ES: events.v1 (via NATS)
            ES->>CH: insert
            ES->>U: SSE push (via api-gateway)
        end
    end
```

This is **the** canonical flow. Every other interaction is a variation
on it.

---

## Lifecycle: a model from training to promotion

```mermaid
sequenceDiagram
    autonumber
    actor D as Data scientist
    participant AS as annotation-service
    participant DS as dataset-service
    participant TO as training-orchestrator
    participant MR as model-registry
    participant CC as canary-controller
    participant PG as policy-gate-service
    actor A as Approver

    D->>AS: label samples
    D->>DS: cut dataset version v3
    D->>TO: train job (base, dataset v3)
    TO->>MR: register model v2 (artifact, ref dist)
    D->>CC: submit canary plan (v2 vs v1, 5% traffic, min 1000 samples)
    Note over CC: Wilson lower bound on outcomes
    CC->>PG: request promotion gate
    PG->>A: approve?
    A->>PG: approve
    PG->>CC: gate.resolved
    CC->>MR: promote v2 to 100% traffic
```

There is **no force-promote endpoint**. ADR-0023.

---

## What's *not* a resource (and why)

- **Frames.** Frames are not platform resources — they are bytes in
  the claim-check store, referenced by URN. Storing a frame as a
  resource would put bytes on the bus (ADR-0008).
- **Inference calls.** Inference is a *per-frame*, *per-event* signal.
  It's a bus message, not a resource.
- **Detections (raw).** Raw detections become events when a rule
  trips. The raw firehose goes to ClickHouse but isn't a
  CRUD-managed resource.

---

## Where next

- [`data-flow.md`](./data-flow.md) — sequence diagrams across the platform.
- [`api-reference.md`](./api-reference.md) — REST endpoints.
- [`glossary.md`](./glossary.md) — terminology.
