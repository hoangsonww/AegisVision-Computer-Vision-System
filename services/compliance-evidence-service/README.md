# compliance-evidence-service

> **Composes per-control evidence from authoritative stores.** Owns no
> data. ADR-0029.

`compliance-evidence-service` is **deliberately stateless**. It does not
store evidence. It assembles evidence on demand from other services
(audit, ESO, Vault, Patroni, ClickHouse, chaos attestations, DR drill
results) via a `Collector` interface.

```mermaid
flowchart LR
    AUD[auditor request] --> CE[compliance-evidence-service]
    CE --> C1[Audit Collector]
    CE --> C2[Vault Collector]
    CE --> C3[Chaos Collector]
    CE --> C4[DR Drill Collector]
    CE --> C5[Backup Collector]
    CE --> C6[SBOM / Cosign Collector]
    C1 --> SRC1[audit-service]
    C2 --> SRC2[Vault]
    C3 --> SRC3[tools/dr-drills attestation]
    C4 --> SRC4[tools/dr-drills attestation]
    C5 --> SRC5[WAL-G manifests]
    C6 --> SRC6[in-cluster registry]
```

Why this shape: storing copies of evidence drifts. Composing on demand
guarantees the evidence is *current* — and the auditor can re-verify
by running the same Collector themselves.

---

## API

- `POST /v1/evidence:bundle` — produce a signed bundle for a control set.
- `GET /v1/controls` — list available controls.
- `GET /v1/controls/{id}` — describe a control.

---

## Control catalogue

Out of the box:

- **SOC 2** Common Criteria (CC1-CC9), Trust Service Criteria (security, availability, processing integrity, confidentiality, privacy).
- **EU AI Act** Annex IV conformity assessment.
- **GDPR DPIA**.
- **CIS Kubernetes Benchmark**.

Each is in `internal/controls/`; adding a new framework is a new
Collector plus a YAML mapping.

---

## See also

- [`docs/compliance/`](../../docs/compliance/) — control narratives and templates.
- ADR-0029.
