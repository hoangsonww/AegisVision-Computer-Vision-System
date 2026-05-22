# DR drill scripts

The platform's RTO commitment (30 min control plane / 5 min data plane)
and RPO commitment (5 min) are only as good as the last drill that
exercised them. This directory owns the runnable scripts the on-call
rotation invokes quarterly to validate both numbers.

Per ADR-0028, the drill output is a signed audit record. SOC 2 CC9.1
and CC7.5 require evidence that DR was exercised; the
`compliance-evidence-service` surface returns it on demand.

## What's in here

```
tools/dr-drills/
├── README.md
├── run-quarterly.sh         # entry point — runs every drill in sequence
├── postgres-restore.sh      # wal-g restore + integrity check
├── clickhouse-restore.sh    # clickhouse-backup restore + integrity check
├── vault-restore.sh         # per-tenant transit-key restore
├── nats-jetstream-replay.sh # replay JetStream from snapshot
└── chaos-attestation.sh     # sign + upload the result as audit record
```

## Running the quarterly drill

Pre-requisites:

- Staging cluster with the production-shape DB / object-storage backends.
- WAL-G + clickhouse-backup + vault CLI on PATH.
- `AEGIS_AUDIT_TOKEN` set (used by the attestation upload).

```sh
./tools/dr-drills/run-quarterly.sh \
  --cluster=staging \
  --postgres-host=postgres-restore.aegis-dr.svc \
  --clickhouse-host=clickhouse-restore.aegis-dr.svc \
  --output=/tmp/dr-drill-$(date +%Y-%m-%d).json
```

The wrapper produces one JSON record per sub-drill with PASS/FAIL,
elapsed wall-clock time, and the RTO budget used. The script exits
non-zero if any sub-drill exceeded its budget.

## Per-drill budgets

| Drill | RTO budget | RPO budget |
| ----- | ---------- | ---------- |
| Postgres restore | 30 min | 5 min (WAL archive lag) |
| ClickHouse restore | 45 min (per shard) | 1 hour (last backup) |
| Vault transit-key restore | 10 min | 0 (key escrow is offline) |
| NATS JetStream replay | 5 min | 0 (durable streams) |

## Don't

- **Don't run against production.** Staging only — the restore scripts
  drop the target DB before restore.
- **Don't skip the attestation upload.** Compliance evidence is the
  product of the drill; an unattested PASS is worthless to an auditor.
- **Don't backdate the attestation.** The `chaos-attestation.sh` rejects
  any record older than the drill start by more than 24 hours.
