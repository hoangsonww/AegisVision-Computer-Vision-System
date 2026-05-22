#!/usr/bin/env bash
# Restore Postgres from the most recent WAL-G base backup + WAL archive,
# verify integrity, and assert the data shape matches the canary table.
set -euo pipefail

HOST=""
CLUSTER=""
S3_PREFIX="${WALG_S3_PREFIX:-s3://aegis-backups/postgres}"

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

# 1. Pull the latest base backup name.
echo "▶ wal-g: listing base backups"
WALG_S3_PREFIX="$S3_PREFIX" wal-g backup-list --pretty
LATEST=$(WALG_S3_PREFIX="$S3_PREFIX" wal-g backup-list 2>/dev/null | tail -1 | awk '{print $1}')
[[ -z "$LATEST" ]] && { echo "no base backup found" >&2; exit 1; }
echo "▶ restoring from base backup: $LATEST"

# 2. Stop the restore-target Postgres + clear PGDATA.
RESTORE_DATA=/var/lib/postgresql/restore
sudo systemctl stop postgresql-restore 2>/dev/null || true
sudo rm -rf "$RESTORE_DATA"
sudo mkdir -p "$RESTORE_DATA"
sudo chown postgres:postgres "$RESTORE_DATA"

# 3. Restore the base backup + replay WAL up to "latest".
sudo -u postgres WALG_S3_PREFIX="$S3_PREFIX" wal-g backup-fetch "$RESTORE_DATA" LATEST
sudo -u postgres tee "$RESTORE_DATA/recovery.signal" > /dev/null < /dev/null
sudo -u postgres tee -a "$RESTORE_DATA/postgresql.conf" > /dev/null <<EOF
restore_command = 'wal-g wal-fetch "%f" "%p"'
recovery_target_action = 'promote'
EOF

# 4. Start Postgres + wait for promotion.
sudo systemctl start postgresql-restore
echo "▶ waiting for promotion"
for i in $(seq 1 120); do
  if sudo -u postgres psql -h "$HOST" -c 'SELECT pg_is_in_recovery();' 2>/dev/null | grep -q 'f'; then
    break
  fi
  sleep 5
done
sudo -u postgres psql -h "$HOST" -c 'SELECT pg_is_in_recovery();' | grep -q 'f' || {
  echo "promotion failed" >&2; exit 1; }

# 5. Verify integrity — the canary table must be present + have rows.
echo "▶ integrity check"
COUNT=$(sudo -u postgres psql -h "$HOST" -t -c "SELECT COUNT(*) FROM aegis_canary.heartbeats WHERE inserted_at > now() - interval '10 minutes';" 2>/dev/null | tr -d ' ')
if [[ -z "$COUNT" || "$COUNT" -lt 1 ]]; then
  echo "✗ canary table missing or stale" >&2
  exit 1
fi
echo "✓ canary table has $COUNT recent rows"

echo "✓ postgres-restore drill passed"
