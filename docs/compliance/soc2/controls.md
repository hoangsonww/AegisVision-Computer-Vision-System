# SOC 2 control mapping

For each Trust Services Criterion below: the platform control(s) that
satisfy it, where the control is implemented in code/config, and where
the auditor finds the evidence.

## CC6 — Logical & physical access

### CC6.1 — Logical access controls

> The entity implements logical access security software, infrastructure,
> and architectures over protected information assets to protect them from
> security events.

- **Network-layer**: Istio Ambient with mTLS STRICT on every service
  (`deploy/helm/*/templates/peerauthentication.yaml`). Cilium default-deny
  NetworkPolicies (`deploy/helm/*/templates/networkpolicy.yaml`).
- **Application-layer**: `auth-proxy` JWT verification (RS256/ES256 only;
  HMAC explicitly excluded per `pkg/llm/safety.go` reasoning extended to
  identity). Tenant isolation enforced in every store (per-tenant scoping
  is part of the audit-test for every `internal/store`).
- **Audit**: every authn/authz decision is published to `audit.v1`. Pull
  via `/v1/evidence?control=CC6.1`.

### CC6.2 — Logical access provisioning

> Prior to issuing system credentials, the entity registers and authorizes
> new internal and external users.

- **External**: tenants onboard via `tenant-service` /v1/tenants with
  tier-default quotas applied. Provisioning is itself a tier-3 action in
  internal staff workflows — gated through `policy-gate-service`.
- **Internal staff**: handled by corporate IdP outside this repo.

### CC6.3 — Logical access removal

> The entity authorizes, modifies, or removes access...

- **Tenant deactivation**: `tenant-service` SoftDelete → publishes
  `tenant.lifecycle.v1` with `tenant.deleted`. Downstream services react
  by:
  - event-service: stops accepting new events for the tenant
  - media-service: marks objects for retention sweep
  - llm-gateway: refuses requests bearing the tenant id
- **Right to be forgotten**: crypto-shred via per-tenant Vault transit
  key (ADR-0011 + `docs/compliance/gdpr.md`).

### CC6.6 — Privileged-action approval

> The entity implements logical access security measures to protect
> against threats from sources outside its system boundaries.

This is where the bounded-autonomy architecture earns its keep. Per
ADR-0014/0017:

- Tier-3 (consequential) tools — `promote_model`, `override_retention`,
  hard-delete tenant — are refused by the agent loop in code; they route
  through `policy-gate-service`.
- Every gate decision is recorded with actor, blast radius, decision
  reason. Pull via `/v1/evidence?control=CC6.6`.

### CC6.7 — Restricted physical access

Cloud-hosted; physical security is delegated to AWS / GCP / Azure SOC 2
reports. We attest to the inheritance via vendor reviews.

### CC6.8 — Prevention or detection of unauthorized software

- Kyverno admission policies reject unsigned images
  (`deploy/k8s/policies/kyverno-baseline.yaml`).
- Cosign keyless signing in CI; SLSA v1 provenance attestations on every
  image.

## CC7 — System operations

### CC7.1 — Detection of configuration vulnerabilities

- Dependabot for Go modules + GitHub Actions (CVE alerts).
- govulncheck against the Go binaries.
- Kyverno admission policies enforce non-root, drop-ALL caps,
  readOnlyRootFilesystem, seccompProfile.

### CC7.2 — Monitoring of system performance

- RED metrics emitted by every service via `pkg/platform/metrics`.
- SLO targets persisted in `slo-watchdog` and evaluated every 60s.
- Burn-rate signals emitted when budget consumption exceeds the
  multi-window rule (Google SRE workbook table 5-7). Pull via
  `/v1/evidence?control=CC7.2`.

### CC7.3 — Security event detection

- audit-service consumes `audit.v1` (append-only).
- `aegis_llm_refusals_total{reason}` surfaces prompt-injection refusals.
- `aegis_authn_decisions_total{outcome}` surfaces auth failures.
- Pull via `/v1/evidence?control=CC7.3`.

### CC7.4 — Incident response

- Runbooks live in `docs/runbooks/`. Each major failure mode has one:
  oncall.md, dr.md, drift-spike.md, canary-rollback.md, agent-incident.md,
  chaos-game-day.md.
- Drift signals from `drift-detection-service` and SLO signals from
  `slo-watchdog` are the leading indicators. Pull via
  `/v1/evidence?control=CC7.4`.

### CC7.5 — Recovery of system operations

- DR drill runbook: `docs/runbooks/dr.md`.
- Quarterly automated DR drills via `tools/dr-drills/run-quarterly.sh`.
- WAL-G + clickhouse-backup, encrypted at rest (SSE-KMS), 90-day
  retention.

## CC8 — Change management

### CC8.1 — Change authorization, design, testing, approval

- **Source control**: every change lands via PR. CODEOWNERS gates the
  `/proto` directory most strictly (ADR-0007).
- **Build**: CI runs go vet + go test -race + go build for every module;
  cosign + SLSA per image; helm-lint + conformance for every
  chart.
- **Approval**: branch protections require PR review.
- **Deploy**: ArgoCD GitOps reconciliation. Manual `helm install` is
  blocked at the cluster level by Kyverno (only ArgoCD's SA may apply
  service manifests).

## CC9 — Risk mitigation (business continuity)

### CC9.1 — Business continuity / DR

- Cross-AZ HA: Patroni Postgres (3 replicas across AZs), Redis Sentinel
  (3 replicas), ClickHouse Operator (3 shards × 2 replicas across AZs).
- RTO target: 30 minutes for the control plane, 5 minutes for the data
  plane (event-service auto-failover).
- RPO target: 5 minutes (WAL-G archive interval).

### CC9.2 — Vendor risk

- Vendor list in `docs/compliance/vendors.md` (out of repo for now;
  inventory tracked by procurement).
- All vendor inputs are pinned by SHA in CI workflows + go.sum.
