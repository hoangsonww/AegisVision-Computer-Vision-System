# SOC 2 readiness

This directory is the externally-shareable evidence package for an AICPA
SOC 2 Type 2 audit. It is organized to mirror the Trust Services Criteria
(TSC) so an auditor can walk top-down: control → policy → evidence.

```
soc2/
├── README.md              (this file — start here)
├── controls.md            (full CC1–CC9 control mapping + how the platform satisfies each)
├── evidence.md            (where the auditor pulls the evidence from)
├── policies.md            (corporate policies referenced by controls)
└── exceptions.md          (compensating controls + accepted residual risks)
```

## Trust Services Criteria covered

| Criterion | Control | Where to look |
| --- | --- | --- |
| **CC1** — Control environment | CC1.1–CC1.5 | corporate policies, board minutes; out of scope of this repo |
| **CC2** — Communication & information | CC2.1–CC2.3 | platform: change-management via PR + ArgoCD audit log |
| **CC3** — Risk assessment | CC3.1–CC3.4 | threat model in `pentest-scope.md`; ML-specific risks in `eu-ai-act/conformity-assessment.md` |
| **CC4** — Monitoring activities | CC4.1–CC4.2 | `slo-watchdog`, `drift-detection-service`, audit log |
| **CC5** — Control activities | CC5.1–CC5.3 | OPA + Kyverno admission, mTLS STRICT, ADR-0014 bounded autonomy |
| **CC6** — Logical & physical access | CC6.1–CC6.8 | `auth-proxy` (JWT), OPA (AuthZ), `policy-gate-service` (CC6.6), tenant isolation |
| **CC7** — System operations | CC7.1–CC7.5 | RED metrics, SLO watchdog, drift, chaos drills (CC7.4) |
| **CC8** — Change management | CC8.1 | release-please + signed images + SLSA provenance + ArgoCD diff |
| **CC9** — Risk mitigation | CC9.1–CC9.2 | DR drills (`tools/dr-drills`), business continuity in `docs/runbooks/dr.md` |

Optional (Type 2) additional criteria:

| Criterion | Notes |
| --- | --- |
| **A1** — Availability | Multi-AZ HA per ADR-0002, k8s PodDisruptionBudgets, Patroni Postgres HA |
| **C1** — Confidentiality | Per-tenant crypto-shred, Vault transit, TLS in transit (mTLS STRICT) |
| **PI1** — Processing integrity | Idempotency keys on mutating endpoints, optimistic concurrency on protobuf-versioned resources |
| **P1–P8** — Privacy | GDPR + EU AI Act mappings in `docs/compliance/{gdpr,eu-ai-act}.md` |

## Evidence collection

Auditors pull evidence via `compliance-evidence-service` (read-only,
tenant-scoped). For an audit window of 2026-01-01 to 2026-04-01:

```sh
curl -sS \
  -H 'X-Aegis-Tenant: <tenant>' \
  -H 'Authorization: Bearer $AUDIT_TOKEN' \
  "https://api.example/v1/evidence.csv?control=CC6.6&from=2026-01-01T00:00:00Z&to=2026-04-01T00:00:00Z"
```

This returns the CC6.6 (privileged-action approval) evidence as CSV — one
row per gate decision, with timestamp, actor, blast radius, decision
reason. The same query is available as JSON (`/v1/evidence`) or JSON-Lines
(`/v1/evidence.jsonl`) for very large windows.

## Type 1 vs Type 2

- **Type 1** — design adequacy. The auditor inspects the architecture,
  policies, control descriptions. This repo (with ADRs + runbooks +
  compliance docs) is the design artifact.
- **Type 2** — operating effectiveness. The auditor samples evidence
  over a 6+ month observation window. `compliance-evidence-service`
  produces that evidence on demand.

The platform was designed to support Type 2 from day one — every
load-bearing control has machine-readable evidence (audit log row,
metrics counter, policy-gate decision row).
