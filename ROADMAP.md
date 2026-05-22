# Roadmap

> **Where AegisVision is going.** This is a living document — please read
> [`CHANGELOG.md`](./CHANGELOG.md) for what's already shipped.

The roadmap is grouped by *theme*, not by service. A theme typically
spans 3–6 services and 1–2 ADRs.

---

## Status today (v0.5.x)

Phases 0–6 complete:

- 35 Go services, 14 shared libraries, 38 Helm charts.
- 30 Architecture Decision Records.
- Compliance evidence: SOC 2, EU AI Act conformity, GDPR DPIA, pen-test scope.
- 10 chaos experiments + 6 DR drill scripts + 3 air-gap scripts + 4 load tests.
- 49 / 49 modules green under `-race`.
- 38 / 38 charts conformance green.
- 5 / 5 integration smoke tests green.

What remains: **operational validation** — a real production-shape
Kubernetes deployment, a real SOC 2 audit, a real EU AI Act
conformity assessment, a real 10k-stream soak.

---

## Near-term (next 1–2 releases)

### Console (Phase 7) — **shipped**

Next.js 14 + TanStack Query + Tailwind console at
[`services/console/`](./services/console). Exposes every public REST
endpoint as a usable page:

- Dashboard with live SSE event feed + KPIs.
- Pipelines (CRUD + revisions + promote).
- Streams (CRUD + pause/resume + per-stream SSE).
- Models (register + versions + gated promotion via policy-gate).
- Datasets, dataset versions, samples, lineage.
- Annotations + label-policy revisions.
- Training jobs (start + cancel + lineage view).
- Media clips + retention policies (redact + refuse-raw-sinks).
- Rules editor (dwell / count / line-cross / zone-enter).
- Events (live SSE + historical search + JSON viewer).
- Agents (chat UI with citations + tier-3 gate banner + auto-resume).
- Approval-gate inbox (tier-3 actions + decision history).
- Canary plans (Wilson lower-bound decision board).
- Shadow inference overview.
- Drift (JS / KL / TVD with threshold-breach alerts).
- SLO burn-rate board (MWMBR).
- Prefetch grid (7×24 EMA heatmap per tenant/model).
- Knowledge RAG query + re-ingest.
- NLQ parser.
- Active-learning queue.
- Semantic search.
- Tenants + projects + members + RBAC.
- Cost + metering rollups + invoices.
- Compliance evidence bundles (SOC 2 / EU AI Act / GDPR / CIS).
- Append-only audit log + chain-verify.
- Settings.

Deploys via the conformance-clean Helm chart at
[`deploy/helm/console/`](./deploy/helm/console). ArgoCD ApplicationSet
picks it up automatically.

### Tenant SDKs

First-class client libraries:

- Python (priority — broad data-science adoption).
- TypeScript.
- Go.

Generated from the same protos. Per-language idiomatic.

### Real-model artifact catalogue

Today: bring your own. Roadmap:

- Public mirror of a few permissively licensed models (YOLO family,
  COCO-trained).
- One-command "import this huggingface model into model-registry."
- A trained "person + vehicle" pipeline for the walking skeleton.

---

## Mid-term

### True multi-cluster

Today: single cluster. Roadmap:

- Tenant-affinity to clusters.
- Cross-cluster event federation.
- Multi-region active/active for SLO purposes.

### Edge fleet management

Today: one edge profile per box. Roadmap:

- Centralised edge fleet view.
- Per-edge configuration profiles.
- Bulk OTA model + pipeline rollout to edge boxes.

### Cost optimisation

- Spot-instance bidding for non-critical inference.
- Auto-scale dataplane-runner shards by demand.
- Per-tenant cost guardrails (refuse work above a budget).

---

## Longer-term

### VLM integration in the data plane

Today: VLM lives behind `llm-gateway` for agentic use. Roadmap:

- Optional VLM operator on the DAG (e.g. for free-form scene
  description).
- Cost / latency budget guards.
- ADR for VLM-in-dataplane.

### On-device inference for edge

NVIDIA Jetson family + Apple Silicon + intel NPU. Each is its own
adapter behind the `Detector` interface (`pkg/dataplane`).

### Federated learning loop

Privacy-preserving cross-tenant model improvements. Substantial ADR
work required.

### Closed-loop active-learning → training → canary → promotion

Today: each step exists, gated by humans. Roadmap:

- Configurable per-tenant policy for "auto-train when N samples
  collected, then propose canary."
- All gates retained (ADR-0014 / 0017).

---

## How priorities are set

The roadmap is set by the project owner ([@hoangsonww](https://github.com/hoangsonww)).
Influences:

- User feedback (GitHub Discussions + issues).
- ADR pressure (a theme has too many "we'd need a new ADR for this" notes).
- Security findings.
- The author's curiosity.

This is a solo project. Pace varies.

---

## What's NOT on the roadmap

- A force-promote endpoint. ADR-0023.
- A second agent runtime. ADR-0022.
- Per-frame Postgres calls. ADR-0001.
- Bytes on the bus. ADR-0008.
- HS\*-signed JWTs.

These are deliberate refusals, not gaps.

---

## Contributing to the roadmap

Open a [GitHub Discussion](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/discussions)
under "Ideas" with:

- The theme.
- The motivation.
- The smallest first step that would demonstrate value.

If it lines up with the project's direction, I'll move it onto the
roadmap.

---

## Contact

**Son Nguyen** — <hoangson091104@gmail.com> · [@hoangsonww](https://github.com/hoangsonww) · [sonnguyenhoang.com](https://sonnguyenhoang.com)
