#!/usr/bin/env bash
# Nightly Postgres base backup via WAL-G. Run as a CronJob in aegis-data;
# the cron schedule lives in deploy/platform/data/postgres-patroni.yaml.
set -euo pipefail

source /etc/aegis/wal-g-config.env

NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "[$NOW] starting base backup"
wal-g backup-push "$PGDATA"
echo "[$NOW] verifying — listing the last 3 backups"
wal-g backup-list | tail -3
echo "[$NOW] cleaning up backups older than 30 days"
wal-g delete retain FULL 30 --confirm
echo "[$NOW] done"
