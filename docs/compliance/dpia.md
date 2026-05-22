# Data Protection Impact Assessment (DPIA)

Per GDPR Art. 35, processing operations involving systematic monitoring
of publicly accessible areas on a large scale, or biometric identification,
require a DPIA before being put into operation. This document is the DPIA
template for an AegisVision deployment.

## 1. Description of the processing

- **Nature**: real-time video analytics, object/person detection,
  optional face/gait identification, optional license-plate
  recognition.
- **Scope**: per tenant, configurable. Defaults exclude facial
  identification and plate recognition unless explicitly enabled.
- **Context**: deployed by operators (employers, retailers, public
  bodies) at locations they control or are authorized to monitor.
- **Purpose**: safety (incident detection), operations (queue / dwell
  analytics), security (intrusion alerts). Marketing surveillance and
  emotion recognition are **out of scope** and the platform refuses to
  load models tagged with those purposes.

## 2. Necessity and proportionality

- **Lawful basis (Art. 6)**: Legitimate interest (Art. 6(1)(f)) for
  operational uses; consent or public-interest task for higher-risk
  uses. Operators document their lawful basis in the tenant's profile.
- **Data minimization (Art. 5(1)(c))**: redaction operator runs by
  policy; the dataplane refuses to emit raw-imagery sinks for tenants
  whose policy requires redaction (compile-time check in
  `pipeline-service`).
- **Storage limitation (Art. 5(1)(e))**: per-tenant TTL on media; default
  30 days, max 5 years (compliance-mode overrides for legal hold).

## 3. Risks to data subjects

| Risk | Likelihood | Severity | Mitigation |
| ---- | ---------- | -------- | ---------- |
| Mis-identification (false positive on face) | Medium | High | Mandatory fairness eval at model promotion; per-class confidence thresholds; human review for high-stakes decisions |
| Unauthorized access to recorded footage | Low | High | mTLS STRICT + per-tenant Vault transit key + Kyverno admission policies |
| Tracking beyond legitimate purpose | Low | Medium | Per-tenant policy (`docs/compliance/gdpr.md` §3) defines allowed inference classes; dataplane enforces at compile time |
| Inability to exercise rights (access/erasure) | Low | High | `tenant-service` DSAR workflow; crypto-shred via Vault key destruction (60-second guaranteed-erasure window) |
| Data exported across borders | Variable | Variable | Tenant chooses region; cross-region replication off by default; logs the export to the audit trail |
| Drift causing degraded accuracy on minority groups | Medium | High | `drift-detection-service` tracks per-class distribution; alerts on shifts; retraining via active-learning loop |

## 4. Measures envisaged

### Technical

- mTLS STRICT (ADR-0014); JWT-validated egress.
- Crypto-shredding via per-tenant Vault transit key.
- Append-only audit log; SOC 2 + EU AI Act evidence query API.
- Bounded-autonomy agent gate (ADR-0017) prevents unilateral
  consequential actions.
- Redaction operator + privacy-by-default DAG validator.

### Organizational

- DPO appointed by the operator (out of repo scope).
- Annual pen-test (`docs/compliance/pentest-scope.md`).
- Quarterly chaos drill (`docs/runbooks/chaos-game-day.md`).
- Privacy training for engineers + operators.
- Data Processing Agreements (DPA) with tenants — template in
  `docs/compliance/dpa-template.md`.

## 5. Consultation

- **Data subjects**: tenants publish per-camera signage with QR code
  linking to the privacy notice.
- **DPO**: signs off on every DPIA-bearing deployment.
- **Supervisory authority**: consultation required when residual high
  risk remains after mitigations — per Art. 36, the operator is
  responsible.

## 6. Sign-off

| Role | Name | Date |
| ---- | ---- | ---- |
| Data Controller |  |  |
| DPO |  |  |
| Platform Owner |  |  |
| AI Compliance Officer |  |  |

## 7. Review cadence

- Annually, and on any of:
  - Model promotion that materially changes inference classes
  - Addition of a new high-risk capability
  - Material change in deployed region / jurisdiction
  - SEV-1/SEV-2 incident affecting privacy
