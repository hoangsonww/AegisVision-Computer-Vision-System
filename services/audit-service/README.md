# audit-service

> **Append-only audit log.** Hash-chained, immutable, fail-closed.
> ADR-0014.

`audit-service` is the system of record for *who did what when*. Every
mutating action in the platform writes a record here; every gate
decision; every model promotion; every retention override; every
tenant deletion.

Records are **hash-chained** — each record's hash includes the
previous record's hash. Tampering is detectable by re-walking the
chain. Verification lives in `tools/audit/`.

---

## API

- `POST /v1/audit:append` — append a record.
- `GET /v1/audit` — query (cursor-paginated).
- `GET /v1/audit/verify` — verify the chain from a given offset.

---

## Failure semantics

ADR-0014 mandates **fail-closed**: if an audit append cannot be
persisted, the upstream operation fails. `policy-gate-service` will
refuse to resolve a gate if it cannot audit the resolution.
`tenant-service` will refuse to crypto-shred a tenant if it cannot
audit the deletion.

This is the difference between "we forgot to log this" (cheap to
ignore) and "the audit trail will not lie" (hard to break).

---

## Storage

Postgres, append-only. No `UPDATE`, no `DELETE` — both forbidden in
the runbook. Reads are partitioned by `(tenant_id, ts)`.

```sql
CREATE TABLE audit (
  id          BIGSERIAL PRIMARY KEY,
  prev_hash   BYTEA NOT NULL,
  hash        BYTEA NOT NULL,
  tenant_id   TEXT NOT NULL,
  actor       TEXT NOT NULL,
  action      TEXT NOT NULL,
  resource    TEXT NOT NULL,
  payload     JSONB NOT NULL,
  ts          TIMESTAMPTZ NOT NULL
);
CREATE INDEX ON audit (tenant_id, ts);
```

---

## Configuration

| Var | Purpose |
| --- | --- |
| `AEGIS_PG_DSN` | Postgres. |
| `AEGIS_AUDIT_SIGN_KEY` | Optional cosign key for periodic chain attestation. |

---

## Metrics

- `aegis_audit_appends_total{tenant,action,status}` — rate.
- `aegis_audit_chain_verifications_total{result}` — verifications.
- `aegis_audit_append_failures_total{reason}` — fail-closed triggers.

---

## See also

- [`tools/audit/README.md`](../../tools/audit/README.md) — chain verifier.
- [`docs/compliance/`](../../docs/compliance/) — auditor's view.
- ADR-0014.
