# tools/scripts

> **Miscellaneous operational scripts.**

| Script | Purpose |
| --- | --- |
| `backup-postgres.sh` | Trigger a WAL-G backup of the Patroni cluster. |
| `backup-clickhouse.sh` | Trigger a `clickhouse-backup` snapshot. |
| `wal-g-config.env` | Environment for WAL-G (KMS key, S3 bucket). |

These are intended to be invoked from cron / CronJob inside the
cluster. See [`docs/runbooks/dr.md`](../../docs/runbooks/dr.md) for
the full BCDR plan.
