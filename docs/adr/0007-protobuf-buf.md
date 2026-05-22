# ADR-0007: Protobuf everywhere, Buf-managed

- Status: Accepted
- Date: 2026-05-17

## Context

A multi-language platform (Go, Rust, Python) with hundreds of cross-service
calls cannot use JSON-and-docs as its contract. We need: language-agnostic
codegen, breaking-change detection in CI, a single source of truth, and a
straightforward path to gRPC and event-bus payloads sharing the same
schemas.

## Decision

- **All cross-service APIs and event payloads** are defined as protobuf in
  `/proto`. This includes Kafka payloads (encoded as protobuf binary, not
  JSON).
- **Buf** manages the directory: lint, breaking-change detection,
  generation via remote plugins.
- Generated Go bindings live in `/proto/gen/go` and are vendored into the
  monorepo (one less moving piece in CI).
- `/proto` has the strictest CODEOWNERS review of any directory in the
  repo: changes to a `.proto` file require the owning team plus the
  Architecture Working Group.

## Consequences

- Adding an endpoint is: edit `.proto`, run `task proto`, implement in the
  service. No hand-written JSON struct in a service should ever model a
  cross-service payload.
- Removing a field requires deprecating it on the schema first; CI rejects
  the removal until N+2 releases later.
- Public REST endpoints are translated *at the gateway* from a JSON
  representation to protobuf; internal services see protobuf exclusively.
