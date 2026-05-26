# Changelog

All notable changes to AegisVision will be documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases below v1.0.0 are pre-GA; minor version bumps may include
breaking changes per semver §4. The first GA release will be v1.0.0.

## [0.5.9](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/compare/v0.5.8...v0.5.9) (2026-05-26)


### Bug Fixes

* **landing:** bump navbar mobile breakpoint from 900px to 1150px ([42a40a4](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/42a40a4626a863ca4acd64e69b32a51dc3248795))


### Documentation

* **readme:** move console screenshot from top to the Production console capability section ([97c73f8](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/97c73f846e64500c07167587f980187223d89adf))
* remove duplicate console screenshot from README ([e211002](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/e211002bd4f9e5a5921b50e18f79015a8f94f651))

## [0.5.8](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/compare/v0.5.7...v0.5.8) (2026-05-26)


### Features

* **console+gateway:** add operator console screenshot, /v1/health + CORS ([671f1cb](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/671f1cbdf288a1cfc16077a96c4ee2a6eb992b6d))
* **console+gateway:** add operator console screenshot, /v1/health + CORS ([eed2092](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/eed20927220bed857f935ff56c23bf30d527419d))

## [0.5.7](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/compare/v0.5.6...v0.5.7) (2026-05-25)


### Bug Fixes

* **ci:** bump golangci-lint to v2.12.2 (go1.26 binary) and clear lint findings ([3ab7697](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/3ab76977d041875897c65f175513c720012724ca))
* **ci:** lint job — drop linter's go target to 1.24, broaden shellcheck excludes ([bf4a9d6](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/bf4a9d61f237902051755d58fdef95bb83dfd9c1))
* production-readiness audit — namespace drift, CI lint, broken links ([038e15f](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/038e15f9f12468dc2b0f8140c8309a1ef9ca9346))

## [0.5.6](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/compare/v0.5.5...v0.5.6) (2026-05-24)


### Bug Fixes

* **airgap:** use absolute path for tarball output ([cf61138](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/cf6113877fce81259af7b5c0a5aee192bc15f8bc))
* harden airgap publish + strip version markers from landing page ([dc7cc7d](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/dc7cc7d2ec30a2b55929be06ba13ef374592f013))

## [0.5.5](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/compare/v0.5.4...v0.5.5) (2026-05-24)


### Bug Fixes

* mobile hamburger nav, drop solo-project copy, wait for image in promote ([92d59cb](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/92d59cb192bab6b523f48937fef083f824d0e420))

## [0.5.4](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/compare/v0.5.3...v0.5.4) (2026-05-24)


### CI

* **proto-lint:** pass GITHUB_TOKEN to buf-setup-action ([9b0d14f](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/9b0d14fd35e74e3d8a2d7f09d564bdf66b4f3ba5))


### Chore

* **release:** bump landing-page version marker to v0.5.4 ([61f649e](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/61f649e592b9f8a35417a7abbe00d44a286b0cf5))

## [0.5.3](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/compare/v0.5.2...v0.5.3) (2026-05-24)


### Bug Fixes

* **release:** retag single aegisvision image instead of per-service matrix ([a47ac39](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/a47ac3933e125d6b66bbffcb04cd4a9453d49530))

## [0.5.2](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/compare/v0.5.1...v0.5.2) (2026-05-24)


### Features

* mobile-responsive landing + README trim + cut v0.5.2 ([7ea2ded](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/7ea2ded455ea7e07800d729025135deb16c56b4e))
* **ui:** mobile-responsive landing page + trim README badges ([8d5a801](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/8d5a801ca3464d2bac2cbfd51f76c903b387c38d))


### Bug Fixes

* **pagination:** use strict base64 decoding to reject tampered cursors ([50fe3df](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/50fe3df2f4ad91b949085e8b8ffc23d6b9a5557f))
* **pagination:** use strict base64 decoding to reject tampered cursors ([b73cbd0](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/b73cbd0a4d05f25b697ab0da750887b616e6e70c))


### Chore

* **release:** bump landing-page version marker to v0.5.2 ([9c5e246](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/9c5e2461a7ead59bfec565295744f06cab8f2b9c))

## [0.5.1](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/compare/v0.5.0...v0.5.1) (2026-05-24)


### Features

* UI polish + console TS fixes + rename workflows ([7e10126](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/7e101268a3a9a3d2c284159985731172a035859d))
* UI polish + console TS fixes + rename workflows ([d900e67](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/d900e679d968d74e433ec603f3b23f0bdacbc50d))


### Bug Fixes

* **docker:** drop pipefail (not in dash); explicit exit 1 on build failure ([d6db333](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/d6db333d40a53c0f5ef7e4b0bfe26e14cf14640a))


### CI

* disable buildx provenance/load on PR (docker exporter rejects manifest lists) ([43de807](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/43de80797386c194c8169e512b5efdc5bd24e8d1))
* drop trivy, collapse 35-image matrix to single aegisvision image, fix console build ([43c5da2](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/43c5da2802a07f0247b3421ee64a0055d97c3bd1))
* fix lint failures across helm/proto/workflows ([6c9399f](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/6c9399f8478a5ede95647d5c762886b96c3fd9a3))
* **helm:** unwrap second escape variant {{"..."}} in 14 more charts ([07d1a48](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/commit/07d1a4828dfe60763d333ad6953bdc3415128f17))

## [Unreleased]

CI cuts entries here automatically via release-please when commits
landing on `main` follow [Conventional Commits](https://www.conventionalcommits.org/).

## [0.5.0] — 2026-05-21 — Operations & compliance hardening

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

## [0.4.0] — 2026-05-21 — Continuous autonomy

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

## [0.3.0] — 2026-05-21 — Intelligence tier

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

## [0.2.0] — 2026-05-21 — Multi-tenant scale & enterprise

### Features

- `tenant-service` with tier-default quotas + DSAR workflow.
- `auth-proxy` JWT verification + HMAC-signed downstream identity.
- `edge-gateway` with disk-backed store-and-forward queue.
- `cost-accounting` with OpenCost + DCGM scrapers.
- Patroni Postgres HA, Redis Sentinel, ClickHouse cluster.
- Per-tenant rate-limit middleware; resource quotas; redaction operator.
- GDPR + EU AI Act + SOC 2 control documents.

## [0.1.0] — 2026-05-21 — Walking skeleton + core platform

### Features

Initial walking skeleton + core platform.
- Foundations: monorepo structure, ADRs, Buf-managed proto, GitOps base.
- Glass-to-event walking skeleton: api-gateway, pipeline-service,
  stream-manager, event-service, dataplane-runner. End-to-end
  glass-to-event ≈ 2.7 ms p95.
- Full 22-service catalog, ClickHouse + Kafka, DAG compiler +
  YAML loader + visual editor, NVIDIA Triton model serving,
  conformance-gated supply chain.
