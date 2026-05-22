# EU AI Act conformity assessment

Per Article 43, providers of high-risk AI systems must conduct a
conformity assessment before placing the system on the EU market. This
document is the conformity-assessment template populated for
AegisVision when configured for high-risk biometric identification or
ML-driven decisioning. It follows Annex IV's structure.

## 1. General description (Annex IV §1)

- **Name**: AegisVision platform
- **Provider**: (operator's legal entity)
- **Intended purpose**: GPU-native real-time visual intelligence —
  detection, tracking, and event generation over live camera feeds.
  Configurations involving face / gait identification or RBI in public
  spaces are classified high-risk; the platform refuses to start the
  high-risk capability unless `AEGIS_COMPLIANCE_HIGH_RISK_ENABLED=true`
  is set with operator sign-off.
- **Form**: software-as-a-service (multi-tenant) + on-prem deployment
  (single-tenant edge bundle, ADR-0027).
- **Categories of use**: workplace safety, supply-chain monitoring,
  public-space situational awareness (only with legal carve-out).
- **EU classification**: high-risk (Annex III §1(a) biometric ID +
  Annex III §6(d) law-enforcement support, if configured).

## 2. Detailed description of elements (Annex IV §2)

- **System architecture**: documented in `docs/adr/` (24 ADRs as of
  Phase 6) and `README.md`.
- **Models**: each model is registered in `model-registry` with full
  provenance (training dataset URN, evaluation metrics, fairness
  evaluation, intended-use statement).
- **Data**: training and validation datasets tracked in
  `dataset-service`. Reference distributions captured in
  `drift-detection-service` at promotion time.
- **Computational resources**: GPU placement is ledger-tracked
  (`gpu-scheduler`, ADR-0003); MIG partitioning bounds blast radius.
- **Human-in-the-loop**: ADR-0014 + ADR-0017 — consequential model
  decisions route through `policy-gate-service`.

## 3. Risk management system (Annex IV §3, Art. 9)

- **Continuous monitoring**: drift signals every 30s, SLO burn signals
  every 60s.
- **Quarterly model review**: scheduled via `autonomy-orchestrator`
  Schedule with role `monitor`, evaluates fairness metrics.
- **Incident database**: `docs/runbooks/incidents/` accepts post-mortems
  for every SEV-1/SEV-2.

## 4. Data and data governance (Annex IV §4, Art. 10)

- **Training data**: representativeness checked per tenant via the
  active-learning loop (ADR-0019).
- **Validation/testing**: held-out split tracked in `dataset-service`
  versions; immutable once captured.
- **Bias**: fairness eval per protected class is required at model
  promotion (`canary-controller` plan must include the fairness check).
- **Personal data**: redaction operator (`pkg/dataplane/operators/redact.go`)
  applies face + plate blurring before storage by policy.

## 5. Technical documentation (Annex IV §5, Art. 11)

This repository is the technical documentation. It is versioned, signed
(SLSA v1), and reproducible (air-gapped bundle is the exact deployable).

## 6. Record-keeping (Annex IV §6, Art. 12)

- All inference operations emit an `inference.completed.v1` audit row
  (model, version, confidence, latency).
- All consequential gate decisions land in `audit.v1` under the
  `policy-gate` category.
- Retention: per tenant configuration, 1–10 years (operator
  jurisdictional default 1 year; high-risk capability forces 5 year
  minimum).

## 7. Transparency and provision of information (Annex IV §7, Art. 13)

- Per-tenant disclosures: every detection event references the model
  ID + version; consumers can verify via `model-registry`.
- Operator-facing: agent responses always cite their sources via
  `knowledge-service` (ADR-0020).

## 8. Human oversight (Annex IV §8, Art. 14)

- **Pre-deployment**: model promotion requires explicit human approval
  via `policy-gate-service`. Refused / un-approved gates cannot promote
  (fail-closed; verified by `chaos:policy-gate-down`).
- **In-operation**: every consequential agent action requires human
  approval. Operators can pause / roll back any canary at any time.
- **Operator controls**:
  - Pause individual pipelines.
  - Revoke a tenant.
  - Disable a model globally.
  - Disable an autonomy schedule.

## 9. Accuracy, robustness, cybersecurity (Annex IV §9, Art. 15)

- **Accuracy**: tracked per (tenant, model) by `event-service` +
  `drift-detection-service`. Promotion-blocking thresholds in the
  canary plan.
- **Robustness**: chaos drills (10 experiments, quarterly).
- **Cybersecurity**: SOC 2 control mapping in `docs/compliance/soc2/`.
  Image signing + SBOM + SLSA v1 provenance + Trivy scan in CI.

## 10. Conformity assessment

Self-assessment per Art. 43(2) (high-risk AI systems other than those
listed in Annex II): the provider conducts the conformity assessment
based on internal control with the technical documentation as above.

### Self-attestation checklist

- [ ] Risk management plan reviewed and approved by AI compliance officer
- [ ] Data governance policies in place + reviewed annually
- [ ] Model promotion gate exercised in the past 30 days
- [ ] Quarterly chaos drill passed (or open SEV-1 for any failure)
- [ ] DR drill passed within the last quarter
- [ ] Fairness evaluation report for every production model less than
      6 months old
- [ ] Post-market monitoring plan documented (handled by
      `autonomy-orchestrator` continuous schedules)

## 11. EU declaration of conformity (Art. 47)

The signed declaration is produced at release time by
`tools/airgap/build.sh` (see `manifest.json` → declaration field).
Auditors verify via `cosign verify` against the platform's public
signing key.

## 12. CE marking (Art. 48)

Applied to the air-gapped bundle's outer label. Procedural; see
`docs/compliance/ce-marking-runbook.md` (operator-supplied for their
specific market).

## 13. Post-market monitoring (Art. 72)

The `autonomy-orchestrator` Schedule with role `monitor` is the
post-market monitoring program in code:

- Hourly: SLO burn-rate review.
- 15-minutely: drift-signal review per model.
- On signal: agent investigates + (if consequential mitigation required)
  opens a gate for the platform owner.

## 14. Serious incident reporting (Art. 73)

The runbook `docs/runbooks/incident-response.md` covers the 15-day
reporting window. The audit log is the evidence record.
