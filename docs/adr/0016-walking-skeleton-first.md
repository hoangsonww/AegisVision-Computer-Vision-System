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

The platform's first deliverable is a **walking skeleton**: one stream →
NVIDIA Triton inference → detections → events → console; control plane
and data plane *actually separated*; claim-check *actually used*; OTel
traces actually flowing from the browser to a Kafka header.

The platform's foundational building blocks (protobuf contracts,
golden-path platform library, the minimum two services on each plane,
deploy manifests, ADRs, CI) exist to make the walking skeleton
achievable.

## Consequences

- Lower per-component fidelity early is acceptable in exchange for
  end-to-end integration earlier.
- New work on a capability does not begin until the walking skeleton it
  depends on is green.
- New services do not get a slot in the GitOps fleet until they have a
  proven path through the gateway, mesh, OPA, and observability stack.
