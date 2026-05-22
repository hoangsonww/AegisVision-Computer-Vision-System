#!/usr/bin/env bash
# Quarterly DR drill entry point. Runs each restore drill in sequence,
# captures wall-clock + PASS/FAIL, and signs the aggregate as an audit
# record (SOC 2 CC9.1 / CC7.5 evidence).
set -euo pipefail

CLUSTER=""
POSTGRES_HOST=""
CLICKHOUSE_HOST=""
OUTPUT=""
SKIP_ATTESTATION=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster=*)        CLUSTER="${1#*=}"; shift ;;
    --cluster)          CLUSTER="$2"; shift 2 ;;
    --postgres-host=*)  POSTGRES_HOST="${1#*=}"; shift ;;
    --postgres-host)    POSTGRES_HOST="$2"; shift 2 ;;
    --clickhouse-host=*) CLICKHOUSE_HOST="${1#*=}"; shift ;;
    --clickhouse-host)  CLICKHOUSE_HOST="$2"; shift 2 ;;
    --output=*)         OUTPUT="${1#*=}"; shift ;;
    --output)           OUTPUT="$2"; shift 2 ;;
    --skip-attestation) SKIP_ATTESTATION=true; shift ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
[[ -z "$CLUSTER" ]] && { echo "missing --cluster" >&2; exit 2; }
[[ -z "$OUTPUT" ]] && OUTPUT="/tmp/dr-drill-$(date +%Y-%m-%d).json"

HERE="$(cd "$(dirname "$0")" && pwd)"
START="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "▶ DR quarterly drill — cluster=$CLUSTER start=$START"
echo "  Output: $OUTPUT"
echo

declare -a RESULTS
overall_pass=true

run_drill() {
  local name="$1"; shift
  local budget_seconds="$1"; shift
  local cmd_start cmd_end elapsed status outcome
  echo "▶ $name (budget ${budget_seconds}s)"
  cmd_start=$(date +%s)
  if "$@"; then status="pass"; outcome=0; else status="fail"; outcome=$?; overall_pass=false; fi
  cmd_end=$(date +%s)
  elapsed=$((cmd_end - cmd_start))
  if [ "$elapsed" -gt "$budget_seconds" ]; then
    status="budget_exceeded"
    overall_pass=false
  fi
  RESULTS+=("$(printf '{"name":%q,"status":%q,"elapsed_seconds":%d,"budget_seconds":%d,"exit_code":%d}' \
    "$name" "$status" "$elapsed" "$budget_seconds" "$outcome")")
  echo "  → $status (elapsed=${elapsed}s, budget=${budget_seconds}s, code=$outcome)"
  echo
}

# ---- Drill 1: Postgres restore (30 min budget) -------------------------
if [[ -n "$POSTGRES_HOST" ]]; then
  run_drill postgres-restore 1800 \
    "$HERE/postgres-restore.sh" --host "$POSTGRES_HOST" --cluster "$CLUSTER"
fi

# ---- Drill 2: ClickHouse restore (45 min budget) -----------------------
if [[ -n "$CLICKHOUSE_HOST" ]]; then
  run_drill clickhouse-restore 2700 \
    "$HERE/clickhouse-restore.sh" --host "$CLICKHOUSE_HOST" --cluster "$CLUSTER"
fi

# ---- Drill 3: Vault transit-key restore (10 min budget) ----------------
run_drill vault-transit-restore 600 \
  "$HERE/vault-restore.sh" --cluster "$CLUSTER"

# ---- Drill 4: NATS JetStream replay (5 min budget) ---------------------
run_drill nats-jetstream-replay 300 \
  "$HERE/nats-jetstream-replay.sh" --cluster "$CLUSTER"

END="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
# Emit aggregate result as JSON.
{
  printf '{'
  printf '"started_at":%q,' "$START"
  printf '"finished_at":%q,' "$END"
  printf '"cluster":%q,' "$CLUSTER"
  printf '"overall":%q,' "$([ "$overall_pass" = "true" ] && echo pass || echo fail)"
  printf '"drills":['
  IFS=,; echo "${RESULTS[*]}"
  printf ']}\n'
} > "$OUTPUT"

# ---- Sign + upload as audit record -------------------------------------
if [[ "$SKIP_ATTESTATION" == "false" ]] && [[ -x "$HERE/chaos-attestation.sh" ]]; then
  "$HERE/chaos-attestation.sh" --input "$OUTPUT" --category "dr.drill.v1"
fi

if [[ "$overall_pass" == "true" ]]; then
  echo "✓ DR drill PASSED — see $OUTPUT"
  exit 0
fi
echo "✗ DR drill FAILED — see $OUTPUT" >&2
exit 1
