# ADR-0008: Claim-check for frames; no media on Kafka or NATS

- Status: Accepted
- Date: 2026-05-17

## Context

The v0.x concept brief defined Kafka topics named `frames.raw`,
`frames.decoded`, and `frames.annotated`. At target scale (10,000 streams
× 1920×1080 × 30 fps × even modest compression) this is hundreds of GB/s of
broker traffic — economically and operationally impossible.

## Decision

Frames and media **never** travel on Kafka or NATS. The bus carries
**references** (claim-checks) to data that lives elsewhere:

- **SHARED_MEMORY** (`/dev/shm` or hugepages ring) — between adjacent
  on-node operators
- **CUDA_IPC** — between GPU-resident operators on the same host
- **OBJECT_STORE** (S3 / MinIO) — for promoted clips, thumbnails, training
  data
- **LOCAL_FILE** — dev / test only

The wire envelope is `aegisvision.dataplane.v1.FrameDescriptor` — a few
hundred bytes after protobuf encoding.

## Consequences

- New operators must accept and emit `FrameDescriptor`. They never marshal
  pixels into a bus payload.
- Cross-node operator placement is preferred to be co-resident; the
  scheduler treats co-location as a positive signal precisely because of
  the claim-check semantics.
- Retention policies for media live with the *store*, not the bus.
- A service that needs to mirror frames into a downstream tool exports them
  via a deliberate egress operator (e.g., into object storage), not by
  inlining bytes into events.
