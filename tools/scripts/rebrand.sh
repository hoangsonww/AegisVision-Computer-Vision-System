#!/usr/bin/env bash
# rebrand.sh — rewrite the AegisVision brand strings to your product's brand.
#
# This is the script behind `task template:rebrand` (see TEMPLATE.md). It
# rewrites three classes of identifier in place:
#
#   - PRODUCT_NAME : the human-readable name ("AegisVision" → "YourProduct").
#                    Used in docs, the landing page, console copy, chart
#                    descriptions, and badge text.
#   - PRODUCT_SLUG : the kebab-case / lowercase slug ("aegisvision" →
#                    "yourproduct"). Used in Helm chart names, namespace
#                    defaults, Docker image repositories.
#   - ENV_PREFIX   : the env-var prefix ("AEGIS" → "YOURPROD"). Used for
#                    every config variable like AEGIS_NATS_URL.
#
# What this script DOES rewrite:
#   - Markdown, HTML, YAML, JSON, TSX/TS, JS, Go source comments and strings.
#   - Helm chart names, Helm values keys that embed the brand.
#   - Env-var names.
#
# What this script DOES NOT rewrite:
#   - The Go module path (github.com/aegisvision). See TEMPLATE.md for the
#     gsed snippet for that.
#   - The ADRs (historical decisions; keep them as-is to preserve context
#     and to satisfy the Apache 2.0 NOTICE requirement).
#   - The LICENSE file (Apache 2.0 requires verbatim retention).
#
# Usage:
#   ./tools/scripts/rebrand.sh [--dry-run] PRODUCT_NAME PRODUCT_SLUG ENV_PREFIX
#
# Examples:
#   ./tools/scripts/rebrand.sh --dry-run YourProduct yourproduct YOURPROD
#   ./tools/scripts/rebrand.sh YourProduct yourproduct YOURPROD

set -eu

DRY_RUN=0
if [ "${1:-}" = "--dry-run" ]; then
  DRY_RUN=1
  shift
fi

if [ $# -ne 3 ]; then
  echo "usage: $0 [--dry-run] PRODUCT_NAME PRODUCT_SLUG ENV_PREFIX" >&2
  echo "  PRODUCT_NAME: human-readable, e.g. YourProduct" >&2
  echo "  PRODUCT_SLUG: lowercase slug, e.g. yourproduct" >&2
  echo "  ENV_PREFIX:   env-var prefix (no underscore), e.g. YOURPROD" >&2
  exit 2
fi

PRODUCT_NAME="$1"
PRODUCT_SLUG="$2"
ENV_PREFIX="$3"

# Sanity-check the slug — must be lowercase kebab-safe.
case "$PRODUCT_SLUG" in
  *[!a-z0-9-]*)
    echo "PRODUCT_SLUG must be lowercase letters, digits, and hyphens." >&2
    exit 2
    ;;
esac
case "$ENV_PREFIX" in
  *[!A-Z0-9_]*)
    echo "ENV_PREFIX must be uppercase letters, digits, and underscores." >&2
    exit 2
    ;;
esac

echo "Rebrand plan:"
echo "  PRODUCT_NAME: AegisVision  -> $PRODUCT_NAME"
echo "  PRODUCT_SLUG: aegisvision  -> $PRODUCT_SLUG"
echo "  ENV_PREFIX:   AEGIS        -> $ENV_PREFIX"
echo

# Pick the right sed in-place flag. macOS / BSD sed needs -i ''; GNU sed
# uses -i without an arg. Prefer gsed when available.
if command -v gsed >/dev/null 2>&1; then
  SED="gsed -i"
else
  case "$(uname)" in
    Darwin|*BSD)
      SED="sed -i ''"
      ;;
    *)
      SED="sed -i"
      ;;
  esac
fi

# Build the list of files to consider. We touch everything except:
#   - .git, node_modules, dist, bin, vendor.
#   - The LICENSE and historical ADRs.
#   - Compiled protobuf bindings (regenerated from /proto on `task proto`).
TARGETS=$(
  find . -type f \
    \( -name '*.md' -o -name '*.html' -o -name '*.yaml' -o -name '*.yml' \
       -o -name '*.json' -o -name '*.toml' -o -name '*.ts' -o -name '*.tsx' \
       -o -name '*.js' -o -name '*.go' -o -name 'Dockerfile' -o -name 'Taskfile.yml' \) \
    ! -path './.git/*' \
    ! -path './node_modules/*' \
    ! -path '*/node_modules/*' \
    ! -path './dist/*' \
    ! -path './bin/*' \
    ! -path './vendor/*' \
    ! -path '*/vendor/*' \
    ! -path './proto/gen/*' \
    ! -path './docs/adr/*' \
    ! -name 'LICENSE'
)

apply() {
  local file=$1 from=$2 to=$3
  if [ "$DRY_RUN" -eq 1 ]; then
    if grep -q -- "$from" "$file" 2>/dev/null; then
      echo "would rewrite: $file ($from -> $to)"
    fi
  else
    if grep -q -- "$from" "$file" 2>/dev/null; then
      $SED "s|${from}|${to}|g" "$file"
      echo "rewrote: $file"
    fi
  fi
}

for f in $TARGETS; do
  # 1) Brand name (case-sensitive so "AegisVision" doesn't accidentally
  #    catch the lowercase "aegisvision" slug).
  apply "$f" "AegisVision" "$PRODUCT_NAME"
  # 2) Slug.
  apply "$f" "aegisvision" "$PRODUCT_SLUG"
  # 3) Env-var prefix. We require an underscore boundary so we don't
  #    rewrite arbitrary substrings.
  apply "$f" "AEGIS_" "${ENV_PREFIX}_"
done

# Helm chart NAME entries reference the slug in Chart.yaml and in the
# chart's own labels. Those are already covered by the slug pass above.

if [ "$DRY_RUN" -eq 1 ]; then
  echo
  echo "Dry run complete. Re-run without --dry-run to apply."
  exit 0
fi

echo
echo "Rebrand complete. Recommended next steps:"
echo "  1) Update the Go module path (see TEMPLATE.md §Fork-and-rename)."
echo "  2) Run \`gofmt -w .\` to format any touched Go files."
echo "  3) Run \`task build && task test\` to confirm nothing broke."
echo "  4) Run \`task template:check\` to validate the adoption checklist."
echo "  5) Commit the rebrand as a single, atomic change."
