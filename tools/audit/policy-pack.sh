#!/usr/bin/env bash
# Convert the policy markdown corpus (docs/compliance/) into a single
# PDF policy pack for the auditor's handover folder. Requires `pandoc`
# + a LaTeX engine on PATH.
set -euo pipefail

OUT="${1:-policy-pack-$(date -u +%Y-%m-%d).pdf}"

if ! command -v pandoc >/dev/null 2>&1; then
  echo "pandoc not installed" >&2; exit 1
fi

cd "$(dirname "$0")/../.."   # repo root

# Gather all compliance + ADR docs that aren't tenant-private.
SOURCES=$(find docs/compliance docs/adr -name '*.md' -not -name 'README.md' | sort)

pandoc \
  --from=gfm \
  --to=pdf \
  --pdf-engine=lualatex \
  --variable=geometry:margin=1in \
  --variable=fontsize=11pt \
  --variable=mainfont="DejaVu Serif" \
  --table-of-contents \
  --metadata title="AegisVision — Compliance Policy Pack" \
  --metadata author="AegisVision Platform Team" \
  --metadata date="$(date -u +%Y-%m-%d)" \
  -o "$OUT" \
  $SOURCES

echo "✓ wrote $OUT ($(wc -l <<< "$SOURCES" | tr -d ' ') source files)"
