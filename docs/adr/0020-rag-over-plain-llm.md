# ADR-0020: Retrieval-augmented agent answers (not plain LLM)

**Status:** Accepted (2026-05-21)

## Context

Agents that answer operator questions about the platform need to ground
their answers in real, current platform documentation — ADRs, runbooks,
incident post-mortems. A plain LLM, even a well-prompted one, hallucinates
file names, function names, model versions, and policy details. The same
agent that confidently quotes a non-existent runbook step is the agent
operators stop trusting.

## Decision

All agent and NL-query answers that reference *how the platform works*
MUST be grounded in retrieved citations from `knowledge-service`. The
service exposes a vector search over the docs corpus; the agent calls
`query_knowledge` as a tool, gets snippets + URIs, and is expected to
include those URIs in its final answer.

The system prompt explicitly says: *"Do not invent identifiers."*

## Consequences

- **Cited answers only for platform-facts questions.** The agent is told
  to cite when relevant; for general LLM tasks (e.g. "summarize this
  detection log") citations are optional.
- **Tenant-tagged docs.** A tenant can ingest its own docs (operator
  notes, runbook overrides) and the agent retrieves them alongside
  platform-global docs for that tenant's questions only.
- **No fine-tuning required.** We avoid the operational cost of keeping a
  tenant-specific fine-tuned model in sync with the docs.
- **Drift detection is straightforward.** When a doc is changed in git,
  the knowledge-service reingest job picks up the change; cited URIs
  remain stable (= file path).

## Rejected alternatives

- **Fine-tune the model on docs.** Rejected — operational complexity,
  loss of citation traceability, and stale answers between fine-tune
  cycles.
- **Long-context dump in the system prompt.** Rejected — the docs corpus
  is hundreds of KB, would dominate every request, and we'd still need
  retrieval to pick what's relevant.
