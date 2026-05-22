#!/usr/bin/env bash
# Restore a Vault transit key from offline escrow + verify an
# encrypt/decrypt round-trip succeeds. Per ADR-0011 the per-tenant
# transit key is the crypto-shred lock — its restorability from escrow
# is the load-bearing assertion of this drill.
set -euo pipefail

CLUSTER=""
TENANT_ID="${TENANT_ID:-aegis-canary-tenant}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster=*) CLUSTER="${1#*=}"; shift ;;
    --cluster)   CLUSTER="$2"; shift 2 ;;
    -h|--help)   grep '^#' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

PATH_PREFIX="aegisvision/tenants/${TENANT_ID}"
ESCROW_DIR="${VAULT_ESCROW_DIR:-/var/lib/aegis/vault-escrow}"

# 1. Verify the escrow blob exists.
if [[ ! -f "$ESCROW_DIR/${TENANT_ID}.b64" ]]; then
  echo "✗ no escrow blob at $ESCROW_DIR/${TENANT_ID}.b64" >&2
  exit 1
fi

# 2. Restore the key from escrow.
echo "▶ restoring transit key for $TENANT_ID"
vault write -force "transit/restore" \
  backup="$(cat "$ESCROW_DIR/${TENANT_ID}.b64")"

# 3. Encrypt + decrypt a probe payload — round-trip must yield the input.
PROBE="aegis-dr-drill-probe-$(date +%s)"
CIPHER=$(vault write -field=ciphertext "transit/encrypt/${TENANT_ID}" \
  plaintext="$(echo -n "$PROBE" | base64)")
PLAIN_B64=$(vault write -field=plaintext "transit/decrypt/${TENANT_ID}" \
  ciphertext="$CIPHER")
PLAIN=$(echo "$PLAIN_B64" | base64 -d)
if [[ "$PLAIN" != "$PROBE" ]]; then
  echo "✗ encrypt/decrypt round-trip mismatch: $PLAIN vs $PROBE" >&2
  exit 1
fi

echo "✓ vault-restore drill passed (probe round-tripped)"
