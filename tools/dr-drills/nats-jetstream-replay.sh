#!/usr/bin/env bash
# Replay a NATS JetStream snapshot into a fresh JetStream and verify
# the message count + last sequence match.
set -euo pipefail

CLUSTER=""
STREAM="${JETSTREAM:-events.v1}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster=*) CLUSTER="${1#*=}"; shift ;;
    --cluster)   CLUSTER="$2"; shift 2 ;;
    -h|--help)   grep '^#' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

# 1. Snapshot the canonical stream's manifest (msg count + last seq).
echo "▶ snapshot manifest"
EXPECTED=$(nats stream info "$STREAM" --json | python3 -c 'import json,sys; s=json.load(sys.stdin)["state"]; print(s["messages"], s["last_seq"])')
EXP_MSGS=${EXPECTED%% *}
EXP_SEQ=${EXPECTED##* }
echo "  expected msgs=$EXP_MSGS last_seq=$EXP_SEQ"

# 2. Restore into a fresh stream.
echo "▶ restoring into ${STREAM}-restore"
nats stream restore "${STREAM}-restore" \
  --backup="${JETSTREAM_BACKUP:-/var/lib/nats/backup}/${STREAM}.tar"

# 3. Compare counts.
ACTUAL=$(nats stream info "${STREAM}-restore" --json | python3 -c 'import json,sys; s=json.load(sys.stdin)["state"]; print(s["messages"], s["last_seq"])')
ACT_MSGS=${ACTUAL%% *}
ACT_SEQ=${ACTUAL##* }
echo "  actual   msgs=$ACT_MSGS last_seq=$ACT_SEQ"

if [[ "$ACT_MSGS" != "$EXP_MSGS" ]] || [[ "$ACT_SEQ" != "$EXP_SEQ" ]]; then
  echo "✗ JetStream replay produced mismatched state" >&2
  exit 1
fi
echo "✓ nats-jetstream-replay drill passed"
