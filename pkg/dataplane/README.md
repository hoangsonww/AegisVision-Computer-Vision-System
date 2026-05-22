# pkg/dataplane

> **The streaming operator runtime + claim-check ring + operators.**

This is the heart of the data plane. `dataplane-runner` imports this
library and wires it up; the library itself does not run as a service.

---

## Contents

| Package | Purpose |
| --- | --- |
| `dag` | Operator DAG runtime (channel-based, bounded buffers). |
| `operators` | The six concrete operators: ingest, sampler, detect, tracker, rule, emit. |
| `claimcheck` | In-memory + object-store claim-check store. |
| (top-level) | `Operator` interface, `Frame` descriptor, `Detector` interface. |

---

## The `Operator` interface

```go
type Operator interface {
    Name() string
    Process(ctx context.Context, in <-chan Frame, out chan<- Frame) error
}
```

A `Frame` is a *descriptor*, not bytes. The bytes live in the
claim-check store; the descriptor carries a `BytesURN`.

---

## Why DAG, not pipeline-of-functions

Two reasons:

1. **Backpressure.** Bounded channels naturally apply backpressure.
   When the detect operator is slow, the sampler drops at the source.
2. **Composition.** Adding a new operator (face-blur, OCR, license-plate
   redactor) is a single file — drop it in, wire it in the DAG builder.

---

## Claim-check

```go
type Store interface {
    Put(ctx context.Context, urn string, bytes []byte) error
    Get(ctx context.Context, urn string) ([]byte, error)
    Delete(ctx context.Context, urn string) error
}
```

Implementations:

- `MemoryRing` — for dev / walking-skeleton.
- `S3Store` — production.

The runner writes bytes on ingest, references by URN through the
DAG, and lets a lifecycle policy GC after retention.

---

## The `Detector` interface

```go
type Detector interface {
    Detect(ctx context.Context, frame Frame, model string) ([]Detection, error)
}
```

Implementations:

- `SyntheticDetector` — dev / walking-skeleton.
- `TritonDetector` — production (lives in `services/inference-router`).

Swapping is one line in `dataplane-runner/main.go`.

---

## Anti-patterns

- **Don't put bytes on the bus.** Use claim-check URNs (ADR-0008).
- **Don't add per-frame Postgres calls.** ADR-0001.
- **Don't add unbounded channel buffers.** Backpressure is a feature.
