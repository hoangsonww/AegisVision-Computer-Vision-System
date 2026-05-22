# ADR-0022: Continuous autonomy — scheduled agents architecture

**Status:** Accepted (2026-05-21)
**Refines:** ADR-0014 (bounded autonomy), ADR-0017 (gate implementation).

## Context

Phase 4 shipped on-demand agents — the operator opens a chat, the agent
runs to completion, the session ends. Phase 5 needs agents that run
continuously: an optimizer that audits cost lines every 24 hours, a
monitor that reacts to drift signals within minutes, a reliability agent
that closes the loop on canary rollbacks.

We need an architecture that:

1. Doesn't violate bounded autonomy. Continuous agents still gate
   consequential actions through `policy-gate-service`.
2. Doesn't grow a second agent runtime. The Phase 4 `agent-service`
   stays as the only runner — `autonomy-orchestrator` just opens
   sessions on it.
3. Survives signal storms without flooding the LLM gateway or the human
   approver queue.

## Decision

Introduce `autonomy-orchestrator` (Phase 5) whose only job is to
translate triggers (cron ticks + bus signals) into agent-service sessions.

- **Schedules.** A `Schedule` resource declares either a cron expression
  *or* a signal subject prefix (`autonomy.signal.drift`,
  `autonomy.signal.slo`, `canary.rollback`), the agent role, max steps,
  and max autonomy tier.
- **Cron scheduler.** `pkg/autonomy.Scheduler` fires entries with bounded
  jitter (default 30s) and an overlap guard (skip a tick if the previous
  run hasn't finished).
- **Signal router.** `pkg/autonomy.SignalRouter` longest-prefix-matches
  inbound bus subjects to registered handlers — drift signals route to
  monitor schedules, canary rollbacks route to reliability schedules.
- **Per-fire session.** Each fire opens a regular `agent-service`
  session with the schedule's role and a generated goal that includes
  the trigger reason. Agent-service handles tools + gates as in Phase 4.
- **Run record.** `AutonomyRun` is persisted with the resulting status
  (`completed` / `awaiting_gate` / `failed`). When a gate resolves,
  agent-service emits its terminal status; the orchestrator reflects it
  back into the run on the next ListRuns call (it doesn't need a hot
  resume path — the gate decision drives the agent-service side).

## Consequences

- **No new bypass of the gate.** Continuous agents inherit the Phase 4
  refusal-in-code. A jailbroken or buggy continuous agent still cannot
  promote a model without a human signature.
- **Signal storms degrade gracefully.** Overlap guards + per-tenant
  agent-service rate limits + LLM-gateway rate limits cap the
  ground-truth pressure. Storms are surfaced in
  `aegis_autonomy_runs_total{status="skipped"}` for SREs.
- **One schedule, one session.** No long-lived "running" agents — they
  start, run, and either complete or block on a gate. Resume is the
  Phase 4 path.
- **Auditable history.** Every fire produces an `AutonomyRun` row and a
  per-step audit trail in agent-service.

## Rejected alternatives

- **Long-lived agent processes.** Rejected — they couple uptime to a
  single goroutine and make audit-log integrity harder.
- **Skipping `agent-service` entirely and running the loop inside the
  orchestrator.** Rejected — duplicates the tool dispatcher + gate
  client + LLM client; one runtime is enough.
- **Treating drift signals as fast HTTP webhooks.** Rejected — signal
  storms would amplify; the bus's at-least-once + per-tenant
  partitioning handles backpressure better.
