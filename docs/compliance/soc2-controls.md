# SOC 2 control mapping

This document maps the SOC 2 Trust Services Criteria to AegisVision controls.
It is the source-of-truth for procurement teams + auditors.

## CC1 — Control environment

| Control | Implementation |
| --- | --- |
| CC1.1 (integrity & ethics) | `docs/runbooks/incident-response.md` blameless postmortem norms |
| CC1.2 (board oversight) | Architecture Working Group sign-off on ADRs |
| CC1.3 (org structure) | `team_topology.md`; service owners declared in CODEOWNERS |
| CC1.4 (HR practices) | Out of platform scope — customer-side |
| CC1.5 (accountability) | Audit log records every consequential action with subject identity |

## CC2 — Communication & information

- Service catalog with owners (`docs/services.md`)
- Runbooks (`docs/runbooks/`)
- Notice-of-change channel in incident postmortems

## CC3 — Risk assessment

- Risk register in the architecture doc §42
- Per-release threat modeling — required for new T0 services
- Quarterly DR drill (see `docs/runbooks/dr.md`)

## CC4 — Monitoring activities

| Control | Implementation |
| --- | --- |
| CC4.1 (continuous monitoring) | Prometheus + Loki + Tempo, Grafana SLO dashboards |
| CC4.2 (auditor access) | Read-only access to audit-service `/v1/audit` per signed NDA |

## CC5 — Control activities

- Kyverno admission policies enforce non-root, signed images, no
  privileged, drop ALL caps (`deploy/k8s/policies/`)
- Conformance test in CI rejects any chart that misses the golden-path
  contract (`tools/conformance/`)
- Cosign keyless signing on every production image

## CC6 — Logical access

- mTLS STRICT on every service (Istio Ambient)
- JWT/JWKS authentication at the gateway
- OPA authorization with deny-by-default and per-tier rate limits
- Vault-managed secrets, External Secrets Operator delivers them to pods
- SPIRE/SPIFFE workload identity

## CC7 — System operations

- Health endpoints + Kubernetes liveness/readiness probes (every service)
- Graceful shutdown (`pkg/platform/shutdown`)
- Postgres HA (Patroni), Redis HA (Sentinel), ClickHouse 3×2 cluster, NATS
  3-replica
- CronJob-driven backups (WAL-G for Postgres, clickhouse-backup)

## CC8 — Change management

- GitOps via ArgoCD — every cluster change is a git commit
- protobuf-everywhere with `buf breaking` check on PRs (ADR-0007)
- Conformance test gating in CI
- SLSA provenance attestation on every image

## CC9 — Risk mitigation

- DR runbook + drill cadence
- Crypto-shredding via per-tenant Vault keys (ADR-0014)
- Bounded autonomy gates on agent-initiated consequential actions
- Cross-region replication: ClickHouse 3-shard MergeTree, Postgres logical
  replication (Phase 3+)

## Availability

- Documented SLOs (architecture doc §5)
- Per-service HPA + PDB (enforced by conformance test)
- Topology-spread constraints across AZs (every deployment)

## Confidentiality

- Encryption at rest: KMS-encrypted EBS, ClickHouse + Postgres on encrypted
  volumes, S3 with SSE-KMS
- Encryption in transit: TLS 1.3 external, mTLS internal
- Vault transit-encryption for tenant-scoped data
- Field-level encryption for biometric/PII via per-tenant keys

## Processing integrity

- Idempotency-Key on every mutating endpoint with 24h replay cache
- RFC 9457 errors with stable codes
- Effectively-once via idempotent consumers + DLQ
