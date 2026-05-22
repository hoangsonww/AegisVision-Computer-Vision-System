#!/usr/bin/env bash
# Pull a per-tenant per-window evidence bundle from compliance-evidence-service.
# Output: a zip with one CSV per control + a manifest + an attestation
# (signed if cosign is available).
set -euo pipefail

TENANT=""; FROM=""; TO=""; OUT=""
BASE_URL="${AEGIS_EVIDENCE_URL:-http://compliance-evidence-service.aegis-system:8520}"
TOKEN="${AEGIS_AUDIT_TOKEN:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tenant=*) TENANT="${1#*=}"; shift ;;
    --tenant)   TENANT="$2"; shift 2 ;;
    --from=*)   FROM="${1#*=}"; shift ;;
    --from)     FROM="$2"; shift 2 ;;
    --to=*)     TO="${1#*=}"; shift ;;
    --to)       TO="$2"; shift 2 ;;
    --out=*)    OUT="${1#*=}"; shift ;;
    --out)      OUT="$2"; shift 2 ;;
    -h|--help)  grep '^#' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
[[ -z "$TENANT" || -z "$FROM" || -z "$TO" || -z "$OUT" ]] && {
  echo "usage: --tenant T --from YYYY-MM-DD --to YYYY-MM-DD --out DIR" >&2; exit 2; }

# Convert dates to RFC3339 (start-of-day UTC).
from_rfc="${FROM}T00:00:00Z"; to_rfc="${TO}T00:00:00Z"
mkdir -p "$OUT"
echo "▶ Pulling evidence for tenant=$TENANT [$from_rfc … $to_rfc] → $OUT"

# Enumerate the controls the service supports.
CONTROLS=$(curl -fsS "${BASE_URL}/v1/controls" \
  -H "X-Aegis-Tenant: $TENANT" \
  ${TOKEN:+-H "Authorization: Bearer $TOKEN"} | \
  python3 -c 'import json,sys; print(" ".join(json.load(sys.stdin)["items"]))')
[[ -z "$CONTROLS" ]] && { echo "no controls reported by service" >&2; exit 1; }

for ctl in $CONTROLS; do
  echo "  ▶ $ctl"
  curl -fsS "${BASE_URL}/v1/evidence.csv?control=${ctl}&from=${from_rfc}&to=${to_rfc}&limit=10000" \
    -H "X-Aegis-Tenant: $TENANT" \
    ${TOKEN:+-H "Authorization: Bearer $TOKEN"} \
    -o "$OUT/${ctl}.csv"
done

# Manifest: which controls, how many rows each, the audit window.
{
  echo "tenant=$TENANT"
  echo "from=$from_rfc"
  echo "to=$to_rfc"
  echo "pulled_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  for ctl in $CONTROLS; do
    rows=$(($(wc -l < "$OUT/${ctl}.csv") - 1))   # minus header
    echo "control=${ctl} rows=${rows}"
  done
} > "$OUT/manifest.txt"

# Cosign-sign the manifest for chain-of-custody.
if command -v cosign >/dev/null 2>&1 && [[ -n "${COSIGN_PRIVATE_KEY:-}" ]]; then
  cosign sign-blob --yes --key env://COSIGN_PRIVATE_KEY "$OUT/manifest.txt" \
    --output-signature "$OUT/manifest.sig"
fi

# Bundle everything into a zip for hand-off.
( cd "$(dirname "$OUT")" && zip -r "$(basename "$OUT").zip" "$(basename "$OUT")" > /dev/null )
echo "✓ evidence bundle: $OUT.zip"
