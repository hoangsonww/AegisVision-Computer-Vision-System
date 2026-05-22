# ADR-0014: Bounded agent autonomy with human gates

- Status: Accepted
- Date: 2026-05-17

## Context

The product positions itself as "autonomous." Customers reading that word in
a procurement document interpret it differently from how we intend to ship
it. An autonomous system that can change production must not be able to do
so unreviewably; both compliance (EU AI Act) and operational risk demand a
hard gate.

## Decision

Agents operate at three explicitly-defined levels:

1. **Read / advisory** — search, analyze, recommend. Autonomous.
2. **Reversible operational** — scale a deployment, prefetch a model, adjust
   sample rate within bounds. Autonomous, audited, guardrail-bounded.
3. **Consequential / irreversible / outward-facing** — promote a model,
   change a safety rule, delete data, notify a customer, spend beyond
   budget. **Requires human approval via a Temporal signal gate.**

Every agent action — at every level — writes an `audit.v1` record.

## Consequences

- The agent runtime carries an identity, role, and policy like any other
  caller. Tools invoked through MCP go through the same OPA decision path
  as a human caller (ADR-013).
- New agent capabilities are classified by level at design time; promotion
  from level 2 to 3 requires explicit project-owner review.
- The Temporal signal gate is part of the deploy workflow, not a separate
  approval system. Pausing an approval pauses the workflow.
