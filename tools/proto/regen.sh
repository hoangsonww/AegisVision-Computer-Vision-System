#!/usr/bin/env bash
# Regenerate protobuf bindings under proto/gen/. Wraps `buf generate` with
# preflight tool checks + lint + breaking-change detection.
set -euo pipefail

cd "$(dirname "$0")/../.."   # repo root

MODE="${1:-generate}"
PROTO_DIR="proto"

if ! command -v buf >/dev/null 2>&1; then
  echo "buf not found. Run tools/proto/install-tools.sh first." >&2
  exit 1
fi

(cd "$PROTO_DIR" && buf lint)
(cd "$PROTO_DIR" && buf format -d) || {
  echo "" >&2
  echo "Proto files need formatting — run 'buf format -w' in $PROTO_DIR." >&2
  exit 1
}

# Breaking-change detection against main, when we're on a non-main branch.
BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo main)
if [ "$BRANCH" != "main" ]; then
  (cd "$PROTO_DIR" && buf breaking --against ".git#branch=main,subdir=$PROTO_DIR" || true)
fi

if [ "$MODE" = "check" ]; then
  echo "✓ proto checks passed (no generation)"
  exit 0
fi

(cd "$PROTO_DIR" && buf generate)

# Run gofmt over the generated tree so downstream go vet doesn't complain
# about stale plugin output.
gofmt -w "$PROTO_DIR/gen/go"

echo "✓ proto bindings regenerated under $PROTO_DIR/gen/"
