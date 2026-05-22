#!/usr/bin/env bash
# Restore ClickHouse from the most recent clickhouse-backup snapshot in
# S3, then verify the events.v1 firehose row count matches the
# pre-snapshot tally within the RPO budget (1h of new rows tolerated as
# acceptable lag).
set -euo pipefail

HOST=""
CLUSTER=""
S3_PREFIX="${CLICKHOUSE_BACKUP_S3_PREFIX:-s3://aegis-backups/clickhouse}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host=*)    HOST="${1#*=}"; shift ;;
    --host)      HOST="$2"; shift 2 ;;
    --cluster=*) CLUSTER="${1#*=}"; shift ;;
    --cluster)   CLUSTER="$2"; shift 2 ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
[[ -z "$HOST" ]] && { echo "missing --host" >&2; exit 2; }

# 1. Identify the latest snapshot.
echo "▶ clickhouse-backup: listing remote snapshots"
LATEST=$(clickhouse-backup list remote 2>/dev/null | tail -1 | awk '{print $1}')
[[ -z "$LATEST" ]] && { echo "no snapshot found" >&2; exit 1; }
echo "▶ restoring snapshot: $LATEST"

# 2. Drop the restore database (idempotency).
echo "▶ resetting restore database"
clickhouse-client --host "$HOST" --query "DROP DATABASE IF EXISTS aegis_restore SYNC"
clickhouse-client --host "$HOST" --query "CREATE DATABASE aegis_restore"

# 3. Restore the snapshot.
clickhouse-backup restore_remote --schema --data --rm "$LATEST" --to-database=aegis_restore

# 4. Compare row counts: the snapshot's recorded count vs the restored
# table's count. Match within budget.
echo "▶ integrity check: row counts"
EXPECTED=$(clickhouse-backup show "$LATEST" 2>/dev/null | awk '/events_v1 rows:/ {print $3; exit}')
ACTUAL=$(clickhouse-client --host "$HOST" --query "SELECT count() FROM aegis_restore.events_v1")
echo "  expected≈ $EXPECTED  actual= $ACTUAL"

if [[ -z "$EXPECTED" ]]; then
  echo "✗ snapshot does not record row count; cannot assert integrity" >&2
  exit 1
fi

# Allow up to 1% drift (clickhouse-backup is consistent-cut, so this
# should be 0 in practice).
DRIFT=$(( EXPECTED > ACTUAL ? EXPECTED - ACTUAL : ACTUAL - EXPECTED ))
THRESHOLD=$(( EXPECTED / 100 ))
[[ "$THRESHOLD" -lt 1 ]] && THRESHOLD=1
if [[ "$DRIFT" -gt "$THRESHOLD" ]]; then
  echo "✗ row drift $DRIFT exceeds threshold $THRESHOLD" >&2
  exit 1
fi

echo "✓ clickhouse-restore drill passed (drift=$DRIFT, threshold=$THRESHOLD)"
