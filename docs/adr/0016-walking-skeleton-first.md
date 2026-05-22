# ADR-0016: Walking skeleton first

- Status: Accepted
- Date: 2026-05-17

## Context

The architecture has many components — Temporal, ClickHouse, Kafka, NATS,
the GPU scheduler, the agent mesh, the LLM gateway. The temptation in a
multi-team build is to develop them in parallel and integrate later. That
strategy systematically defers the *integration* risk to the worst possible
moment.

## Decision

Phase 1 is a **walking skeleton**: one stream → Triton inference →
detections → events → console; control plane and data plane *actually
separated*; claim-check *actually used*; OTel traces actually flowing from
the browser to a Kafka header.

Every Phase 0 building block (this commit) exists to make Phase 1
achievable: protobuf contracts, golden-path platform library, minimum two
services, deploy manifests, ADRs, CI.

## Consequences

- We will accept lower per-component fidelity early in exchange for
  end-to-end integration earlier.
- Teams may not start their second feature until the walking skeleton
  reaches Phase 1 exit criteria.
- New services do not get a slot in the GitOps fleet until they have a
  proven path through the gateway, mesh, OPA, and observability stack.
