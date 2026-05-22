#!/usr/bin/env bash
# Sign + upload a chaos / DR drill result as an audit record. Per
# ADR-0028 + ADR-0029 this attestation is the SOC 2 CC9.1 / CC7.5
# evidence; auditors retrieve it via /v1/evidence?control=CC9.1.
set -euo pipefail

INPUT=""
CATEGORY="dr.drill.v1"
AUDIT_URL="${AEGIS_AUDIT_INGEST_URL:-http://audit-service.aegis-system:8092/v1/audit}"
TOKEN="${AEGIS_AUDIT_TOKEN:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --input=*)    INPUT="${1#*=}"; shift ;;
    --input)      INPUT="$2"; shift 2 ;;
    --category=*) CATEGORY="${1#*=}"; shift ;;
    --category)   CATEGORY="$2"; shift 2 ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
[[ ! -f "$INPUT" ]] && { echo "no --input file" >&2; exit 2; }

# Reject backdated attestations (> 24h drift).
START=$(grep -oE '"started_at":"[^"]+"' "$INPUT" | head -1 | cut -d'"' -f4 || true)
if [[ -n "$START" ]]; then
  drift=$(( $(date -u +%s) - $(date -u -d "$START" +%s 2>/dev/null || echo 0) ))
  if (( drift > 86400 )); then
    echo "✗ attestation older than 24h (drift=${drift}s) — refusing to upload" >&2
    exit 1
  fi
fi

# Sign with cosign if available (keyless OIDC in CI; key-based locally).
if command -v cosign >/dev/null 2>&1; then
  if [[ -n "${COSIGN_PRIVATE_KEY:-}" ]]; then
    cosign sign-blob --yes --key env://COSIGN_PRIVATE_KEY "$INPUT" \
      --output-signature "${INPUT}.sig" --output-certificate "${INPUT}.crt"
  else
    cosign sign-blob --yes "$INPUT" \
      --output-signature "${INPUT}.sig" 2>/dev/null || true
  fi
fi

# Upload as an audit record. The audit-service POST /v1/audit is
# admin-only (X-Aegis-Role: platform-admin).
ID="drill-$(date -u +%Y%m%d%H%M%S)-$(shasum -a 256 "$INPUT" | cut -c1-8)"
RECORD=$(printf '{"id":%q,"at":%q,"service":"dr-drill","category":%q,"outcome":"%s","raw":%q}' \
  "$ID" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$CATEGORY" \
  "$(grep -oE '"overall":"[^"]+"' "$INPUT" | head -1 | cut -d'"' -f4)" \
  "$(cat "$INPUT" | tr -d '\n')")

curl -fsS -X POST "$AUDIT_URL" \
  -H 'Content-Type: application/json' \
  -H 'X-Aegis-Role: platform-admin' \
  ${TOKEN:+-H "Authorization: Bearer $TOKEN"} \
  -d "$RECORD" > /dev/null

echo "✓ attestation uploaded: $ID"
