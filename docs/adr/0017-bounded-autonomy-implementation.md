# ADR-0017: Bounded-autonomy implementation

**Status:** Accepted (2026-05-21)
**Supersedes:** none. **Refines:** ADR-0014.

## Context

ADR-0014 committed to bounded autonomy — agents may act on read-only,
advisory, and reversible operations within guardrails; any consequential
or outward-facing action requires a human approval through a Temporal-style
signal gate.

The intelligence tier makes this real. We need an actual policy gate,
an actual agent runtime, and an actual integration between the two —
without backsliding on the principle that *"an autonomous platform that
can change production must not be able to do so unreviewably"*.

## Decision

We adopt a three-tier risk model encoded directly into the LLM tool schema:

| Tier | Name | Examples | Agent behaviour |
|------|------|----------|-----------------|
| 0    | read | `search_detections`, `get_pipeline`, `query_knowledge` | auto-execute |
| 1    | advise | `propose_dag_change`, `propose_model_swap` | auto-execute (produces a draft; never side-effects) |
| 2    | reversible | `pause_draft_pipeline` | auto-execute |
| 3    | consequential | `promote_model`, `override_retention` | **refuse to execute**, open a gate via policy-gate-service |

The risk tier is part of `llm.ToolSchema` (proto-defined). The agent loop
in `pkg/agent` enforces the cap: when a model emits a tool call whose
schema's `risk_tier` exceeds the session's `MaxAutonomyTier`, the agent
calls `Gateway.OpenGate(...)` and halts, returning `awaiting_gate`.

The gate is materialized as a database row in policy-gate-service with
decision="pending" + an audit trail. Operators see it via the console
(GET /v1/gates) and decide via POST /v1/gates/{id}/decision. On decision,
the service emits `gate.resolved.<gate_id>` on the bus; agent-service
subscribes and resumes the awaiting session via `Resume()`, feeding the
human's chosen result back into the LLM context as a tool result.

We deliberately model the gate as a database row plus a bus signal — not
as a Temporal signal — for the initial implementation. A later
migration may move this into a proper Temporal workflow (same external
API, durable timer for expiry).

## Consequences

- **Refusal is enforced in code, not in the prompt.** A jailbroken LLM
  cannot bypass the gate — the agent never invokes a tier-3 tool's `Run`.
  The tier-3 tools' `Run` methods themselves return errors immediately, as
  a second line of defence.
- **Gate decisions are auditable.** Every state change is published to
  `audit.v1`; audit-service appends append-only.
- **No tenant-cross-tenant gates.** Agents are tenant-scoped; gates carry
  the originating tenant id; cross-tenant reads are forbidden.
- **Expiry.** A pending gate older than the policy's TTL becomes "expired"
  on the next sweep; resumed sessions see a refusal result.

## Rejected alternatives

- **"Just let the agent execute and roll back on disagreement."**
  Rejected — rollback on irreversible actions is not a thing (you cannot
  un-delete a model that downstream tenants have now adopted).
- **"Make the LLM responsible for asking."** Rejected — the prompt isn't
  policy. We treat the LLM as adversarial for safety purposes (ADR-0021).
- **"Approve in-line in the agent UI without a gate service."** Rejected —
  the gate is the source of truth + audit boundary; UIs are clients.
