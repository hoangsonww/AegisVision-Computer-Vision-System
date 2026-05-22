# GDPR posture

The platform processes camera imagery, which under GDPR Art. 4(1) is
personal data; faces are special-category data under Art. 9. This document
maps GDPR articles to platform controls.

## Lawful bases

| Activity | Lawful basis | Where enforced |
| --- | --- | --- |
| Operating cameras at a deployment site | Legitimate interest / contract with customer | Tenant onboarding contract |
| Face / person detection | Customer's lawful basis (varies) | Per-tenant `enabled_capabilities` |
| Face recognition (not face detection) | Explicit consent, separately licensed | `MODEL_KIND_FACE` is gated by a tier flag |
| Storing audit logs | Legal obligation + legitimate interest | audit-service retention policy |

## Data subject rights — implementation

### Right of access (Art. 15)

1. Customer submits a DSAR via `POST /v1/dsar` with `kind=access` and the
   subject identifier.
2. `tenant-service` records the request, publishes `tenant.dsar.v1`.
3. Each consuming service (event-service, audit-service, semantic-search,
   media-service) listens for `kind=access` and emits its slice of the
   subject's data to a per-DSAR object-storage prefix.
4. The orchestrator merges the slices into a single ZIP, signs it (cosign),
   and calls `POST /v1/dsar/{id}:complete` with the artifact URI.
5. The customer's data controller can download it through a pre-signed URL.

### Right to erasure (Art. 17)

1. Customer submits `POST /v1/dsar` with `kind=erasure`.
2. The platform executes the cascade:
   - Drop tenant-scoped ClickHouse partitions.
   - Delete media in `s3://aegis-media/<tenant>/`.
   - Delete vector embeddings in semantic-search (Qdrant collection drop).
   - **Crypto-shred**: destroy the per-tenant Vault transit key — any
     encrypted artifact retained for legal hold becomes unrecoverable.
3. Audit records are NOT deleted (legal obligation). They reference the
   tenant ID but no PII beyond what was in `extra`.
4. The erasure workflow itself writes an `audit.v1` event (`TenantErased`)
   so the cascade is itself auditable.

### Right to data portability (Art. 20)

Same flow as Right-of-Access; the export bundle is in a machine-readable
format (JSON + CSV manifests in a ZIP).

### Right to object (Art. 21)

Implemented at the tenant level: the tenant suspends the relevant
pipeline. The platform itself doesn't process pre-recorded data for
subjects it doesn't have a relationship with.

## Privacy by design (Art. 25)

- **Processing at the edge**: imagery never crosses the WAN in cleartext if
  the deployment uses edge-gateway. Only structured detections / events
  leave the site.
- **Retention**: media TTLs are per-stream, shortest-wins. The default is
  conservative; customers explicitly extend.
- **Redaction**: the platform ships a face/plate redaction operator
  (`pkg/dataplane/operators/redact.go`). For tenants whose policy demands
  redaction, pipeline-service refuses to compile a DAG that emits unredacted
  media.
- **Crypto-shredding**: per-tenant Vault transit key. Destruction of the
  key renders all encrypted bytes for that tenant unreadable, regardless
  of where they sit (including backups).

## Records of processing activities (Art. 30)

A `ropa.md` is generated per tenant from the DSAR + audit pipeline. It
lists: data categories, purposes, recipients, transfers, retention, security
measures.

## Data protection impact assessment

For new high-risk capabilities (face recognition, behavior prediction,
automated decision-making), the platform requires a signed DPIA. The signed
DPIA is stored alongside the model artifact in the model registry; the
`fairness_eval_uri` requirement for face models exists for this reason.
