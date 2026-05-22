#!/usr/bin/env bash
# Report which SOC 2 controls have actually produced evidence rows over
# a recent window. A control with ZERO rows is a "covered on paper" gap
# that the auditor will catch — surface it before the auditor does.
set -euo pipefail

TENANT=""; WINDOW="90d"
BASE_URL="${AEGIS_EVIDENCE_URL:-http://compliance-evidence-service.aegis-system:8520}"
TOKEN="${AEGIS_AUDIT_TOKEN:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tenant=*) TENANT="${1#*=}"; shift ;;
    --tenant)   TENANT="$2"; shift 2 ;;
    --window=*) WINDOW="${1#*=}"; shift ;;
    --window)   WINDOW="$2"; shift 2 ;;
    -h|--help)  grep '^#' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
[[ -z "$TENANT" ]] && { echo "missing --tenant" >&2; exit 2; }

# Parse window: 30d / 90d / 365d / etc.
days="${WINDOW%d}"
[[ -z "$days" || ! "$days" =~ ^[0-9]+$ ]] && { echo "bad window (expect like '90d')" >&2; exit 2; }

# macOS date vs GNU date.
if date -u -d "1 day ago" >/dev/null 2>&1; then
  FROM=$(date -u -d "${days} days ago" +%Y-%m-%dT00:00:00Z)
else
  FROM=$(date -u -v-"${days}"d +%Y-%m-%dT00:00:00Z)
fi
TO=$(date -u +%Y-%m-%dT%H:%M:%SZ)

CONTROLS=$(curl -fsS "${BASE_URL}/v1/controls" \
  -H "X-Aegis-Tenant: $TENANT" \
  ${TOKEN:+-H "Authorization: Bearer $TOKEN"} | \
  python3 -c 'import json,sys; print(" ".join(json.load(sys.stdin)["items"]))')

printf '%-12s %-10s %s\n' "control" "rows" "status"
printf '%-12s %-10s %s\n' "-------" "----" "------"
gaps=0
for ctl in $CONTROLS; do
  rows=$(curl -fsS "${BASE_URL}/v1/evidence?control=${ctl}&from=${FROM}&to=${TO}&limit=1" \
    -H "X-Aegis-Tenant: $TENANT" \
    ${TOKEN:+-H "Authorization: Bearer $TOKEN"} | \
    python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("items",[])))')
  status="OK"
  if [[ "$rows" -eq 0 ]]; then
    status="GAP (no evidence in window)"; gaps=$((gaps+1))
  fi
  printf '%-12s %-10s %s\n' "$ctl" "$rows" "$status"
done

if [[ "$gaps" -gt 0 ]]; then
  echo ""
  echo "✗ $gaps control(s) have no evidence in the past $WINDOW — investigate before audit kickoff"
  exit 1
fi
echo ""
echo "✓ every control produced evidence in the past $WINDOW"
