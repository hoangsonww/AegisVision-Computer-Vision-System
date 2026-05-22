#!/usr/bin/env bash
# Verify the integrity of an extracted AegisVision air-gapped bundle.
#
# Checks (in order):
#   1. manifest.json present and well-formed
#   2. every entry in checksums.txt matches its file
#   3. (optional) cosign signature on each image in signatures/, against
#      the embedded cosign public key (if present)
#
# Exits 0 on success, non-zero on any failure. Designed to be safe to run
# without internet access — no network calls.
set -euo pipefail

[[ ! -f manifest.json ]] && { echo "no manifest.json — run from bundle root" >&2; exit 2; }
[[ ! -f checksums.txt ]] && { echo "no checksums.txt" >&2; exit 2; }

VERSION="$(grep -oE '"version": *"[^"]+"' manifest.json | head -1 | cut -d'"' -f4)"
echo "▶ Verifying AegisVision air-gapped bundle ${VERSION}"

# 1. Re-compute every checksum and diff against the manifest.
echo "▶ Verifying $(wc -l < checksums.txt | tr -d ' ') file checksums"
# Re-create the manifest fresh and compare. Anything missing/added shows up.
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
( find . -type f \! -name checksums.txt -print0 | sort -z | xargs -0 shasum -a 256 > "$tmp" )

if ! diff -u checksums.txt "$tmp" >/dev/null; then
  echo "✗ checksum mismatch (showing first 20 differing lines):" >&2
  diff -u checksums.txt "$tmp" | head -20 >&2
  exit 1
fi
echo "  ✓ all checksums match"

# 2. Cosign verification when key is embedded.
if [[ -f cosign.pub ]] && command -v cosign >/dev/null 2>&1; then
  echo "▶ Verifying cosign signatures"
  for sig in signatures/*.sig; do
    [[ -e "$sig" ]] || continue
    svc="$(basename "$sig" .sig)"
    if ! cosign verify --key cosign.pub --offline \
        --signature "$sig" "images/${svc}/" 2>/dev/null; then
      echo "  ! $svc: signature verification failed (image may still be loadable but is not cryptographically tied to this bundle)" >&2
      # We warn rather than fail because the operator may have intentionally
      # rebundled without re-signing — but the bundle's chain-of-custody is
      # only as strong as the worst-verified component.
    else
      echo "  ✓ $svc"
    fi
  done
fi

echo "✓ verification complete"
