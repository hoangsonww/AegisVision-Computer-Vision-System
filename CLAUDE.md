# Working in this repo (notes for Claude Code)

## Repo identity

This is the AegisVision monorepo — an open computer-vision platform
**template** that any organization can fork and operate as the backbone
of their own perception product. The canonical reference is the
architecture in [`ARCHITECTURE.md`](./ARCHITECTURE.md) and the ADRs in
[`docs/adr/`](./docs/adr/).

The repo ships 35 Go services + 1 Next.js console, shared
platform/llm/agent/embeddings/canary/autonomy libs, protobuf contracts,
deploy manifests (39 Helm charts including a hardened NVIDIA Triton
chart), and 30 ADRs. Compliance evidence packages for SOC 2, EU AI Act,
GDPR DPIA, and a pen-test scope live under `docs/compliance/`.

**Public framing rule.** AegisVision is presented as an
adoptable template, not as a phased delivery plan. Do not introduce
"Phase N" language into user-facing docs, READMEs, the landing page, or
the console — keep that vocabulary out of anything an adopter or
auditor reads. Internal references to specific historical milestones
should use the capability area names (foundations, walking skeleton,
GPU hot path, multi-tenant + edge + storage, intelligence tier,
adaptive autonomy, operations & compliance, production console).

## Commands

- `task build` — build every Go module in the workspace
- `task test` — run unit tests with `-race`
- `task vet` — `go vet` across the workspace
- `task proto` — regenerate protobuf bindings via Buf
- `task run:api-gateway` / `task run:pipeline-service` — local dev servers

## Conventions

- **Public API uses product nouns**, never internal model names. Path
  segments are plural. IDs are opaque strings.
- **Errors are RFC 9457 `application/problem+json`** — use `platform/problem`
  to construct them.
- **Pagination is cursor-based**, never offset. Use `platform/pagination`.
- **Every mutating endpoint** must accept `Idempotency-Key`. Wire the
  middleware in `platform/middleware`.
- **Logging is structured (`slog`)**, with `tenant_id`, `request_id`, and
  `trace_id` propagated through context.
- **Metrics follow RED** (rate / errors / duration). Use `platform/metrics`.
- **No frames on the bus.** If you're about to put image bytes on Kafka or
  NATS, stop — that's ADR-008.
- **Don't run per-frame work on Temporal.** Temporal is control-plane only
  (ADR-001).
- **Agents must not auto-execute consequential actions.** Tier-3 tools
  (`promote_model`, `override_retention`, etc.) route through
  `policy-gate-service` for human approval (ADR-0014/0017).
- **No raw LLM calls — talk to `llm-gateway`.** Centralized safety + token
  accounting + rate limit live there (ADR-0018).
- **Platform-fact answers must cite knowledge-service.** No hallucinated
  identifiers (ADR-0020). The agent's `query_knowledge` tool returns
  cited snippets.
- **Continuous autonomy goes through agent-service.** Don't grow a second
  agent runtime in autonomy-orchestrator (ADR-0022). Each schedule fire
  opens a regular agent session — gates still bind.
- **Canary promotion is gated; rollback is automatic.** Don't add a
  force-promote endpoint to canary-controller; promotion goes through
  policy-gate-service (ADR-0023).
- **Shadow inference never publishes a tenant-facing detection.** Only
  observations + metrics (ADR-0024).

## When adding a new service

1. Copy the structure of `services/pipeline-service` (smallest reference).
2. Define its API in `/proto` first (ADR-007). Run `task proto`.
3. Wire `pkg/platform` for logging/OTel/metrics/health/shutdown.
4. Add a Helm chart under `deploy/helm/<service>` (mTLS STRICT, OPA
   AuthorizationPolicy, NetworkPolicy default-deny, ServiceMonitor, HPA).
5. Add it to `go.work`.
6. Add it to the ArgoCD ApplicationSet in `deploy/argocd/`.

## What not to do here

- Don't add a new transport without first defining its protobuf contract.
- Don't bypass `pkg/platform` for "just this one log line" or "just this one
  metric" — the golden path is uniform on purpose.
- Don't add cross-service in-process state. Services share state only through
  declared APIs and the event backbone.
- Don't introduce a new dependency without checking it's not already covered
  by `pkg/platform` or another internal lib.
