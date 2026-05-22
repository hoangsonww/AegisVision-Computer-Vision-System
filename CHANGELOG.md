# Changelog

All notable changes to AegisVision will be documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases below v1.0.0 are pre-GA; minor version bumps may include
breaking changes per semver §4. The first GA release will be v1.0.0.

## [Unreleased]

CI cuts entries here automatically via release-please when commits
landing on `main` follow [Conventional Commits](https://www.conventionalcommits.org/).

## [0.5.0] — 2026-05-21 — Phase 6: GA hardening

### Features

- Air-gapped bundle builder (`tools/airgap/`) — single self-contained
  tarball with images, charts, manifests, signed cosign attestations.
  Per ADR-0027.
- Chaos engineering harness (`deploy/chaos/`) — 10 experiments covering
  every load-bearing failure mode, each with a check job that asserts
  the expected behaviour. Per ADR-0028.
- Load-test SLO gates (`tools/loadtest/streams-10k.js`,
  `agents-1k.js`, `slo-gate.js`) — k6 scripts that enforce the
  GA-scale gate (10k streams, glass-to-event p95 < 300ms).
- `compliance-evidence-service` — read-only auditor query API; composes
  per-control evidence from audit-service, policy-gate-service,
  slo-watchdog, drift-detection-service. Per ADR-0029.
- Release automation via release-please + automated air-gap bundle
  attachment + multi-arch image promotion. Per ADR-0030.

### Documentation

- SOC 2 readiness package (`docs/compliance/soc2/`) — full CC1–CC9
  control mapping + evidence collection procedure.
- EU AI Act conformity assessment template (`docs/compliance/eu-ai-act/`)
  per Annex IV — every section populated for platform's high-risk
  configuration.
- Penetration test scope + STRIDE threat model
  (`docs/compliance/pentest-scope.md`).
- GDPR DPIA template (`docs/compliance/dpia.md`).
- Quarterly chaos game-day runbook (`docs/runbooks/chaos-game-day.md`).
- DR drill scripts (`tools/dr-drills/`) — runnable Postgres + ClickHouse
  + Vault restore drills.

## [0.4.0] — 2026-05-21 — Phase 5: Continuous autonomy

### Features

- `autonomy-orchestrator` — schedules continuous agent runs via cron
  + bus signals; per ADR-0022.
- `canary-controller` — Wilson-bound regression detector + ladder state
  machine; promotion gated, rollback automatic. Per ADR-0023.
- `shadow-inference-service` — runs candidate models on the same frame
  URN without affecting tenant traffic. Per ADR-0024.
- `drift-detection-service` — KL/JS/TVD divergence vs reference; emits
  `autonomy.signal.drift.v1`. Per ADR-0025.
- `slo-watchdog` — Google SRE multi-window burn-rate alerting; emits
  `autonomy.signal.slo.v1`.
- `prefetch-service` — 7×24 EMA grid predicts model demand + warms
  inference-router. Per ADR-0026.
- `pkg/canary` + `pkg/autonomy` shared libraries.

## [0.3.0] — 2026-05-21 — Phase 4: Intelligence

### Features

- `llm-gateway` — uniform OpenAI-compatible front for LLM/VLM backends.
  Per ADR-0018.
- `agent-service` — bounded-autonomy agent runtime; tier-3 tools route
  through gates. Per ADR-0017.
- `policy-gate-service` — human-approval gate for consequential agent
  actions.
- `knowledge-service` — pgvector-backed RAG corpus for cited answers.
  Per ADR-0020.
- `active-learning-service` — uncertainty + diversity sampling; closes
  the loop on labeled samples. Per ADR-0019.
- `nlq-service` — natural-language → SQL/DAG translator with hardening
  guards (read-only, table whitelist, tenant pin).
- Prompt-injection defense-in-depth (sanitizer + PII redactor +
  refusal threshold). Per ADR-0021.

## [0.2.0] — 2026-05-21 — Phase 3: Scale & enterprise

### Features

- `tenant-service` with tier-default quotas + DSAR workflow.
- `auth-proxy` JWT verification + HMAC-signed downstream identity.
- `edge-gateway` with disk-backed store-and-forward queue.
- `cost-accounting` with OpenCost + DCGM scrapers.
- Patroni Postgres HA, Redis Sentinel, ClickHouse cluster.
- Per-tenant rate-limit middleware; resource quotas; redaction operator.
- GDPR + EU AI Act + SOC 2 control documents.

## [0.1.0] — 2026-05-21 — Phase 0–2

### Features

Initial walking skeleton + core platform.
- Phase 0: monorepo structure, ADRs, Buf-managed proto, GitOps base.
- Phase 1: api-gateway, pipeline-service, stream-manager, event-service,
  dataplane-runner. End-to-end glass-to-event ≈ 2.7 ms p95.
- Phase 2: full 22-service catalog, ClickHouse + Kafka, DAG compiler +
  YAML loader + visual editor, Triton model serving, conformance-gated
  supply chain.
