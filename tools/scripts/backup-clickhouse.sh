#!/usr/bin/env bash
# Per-shard ClickHouse backup using clickhouse-backup. Production schedule:
# daily incremental + weekly full, retention 90 days. The script runs on
# each ClickHouse pod via a Job spawned by the ClickHouse Operator's
# scheduled-task CRD.
set -euo pipefail

CONFIG=/etc/clickhouse-backup/config.yml
NOW=$(date -u +%Y%m%d-%H%M%S)
NAME="aegis-${NOW}"

echo "creating local backup: $NAME"
clickhouse-backup --config="$CONFIG" create "$NAME"

echo "uploading to s3://aegis-clickhouse-backups/"
clickhouse-backup --config="$CONFIG" upload "$NAME"

echo "removing local copy"
clickhouse-backup --config="$CONFIG" delete local "$NAME"

echo "pruning remote backups older than 90 days"
clickhouse-backup --config="$CONFIG" delete remote --keep=90
