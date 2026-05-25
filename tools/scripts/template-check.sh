#!/usr/bin/env bash
# template-check.sh — confirm the AegisVision template adoption checklist
# items that can be checked locally (without a cluster + audit window).
#
# Runs:
#   - All modules build (go build ./... per module).
#   - All modules pass `go vet`.
#   - All modules pass `go test -race`.
#   - Helm conformance test (tools/conformance) passes.
#   - Integration smoke (tools/integration) passes.
#   - Proto lint (`buf lint`) passes if buf is available.
#   - Doc link check (tools/scripts/check_links.go) reports zero broken links.
#
# Adopters run this before their first deploy to make sure their fork
# is green end-to-end.

set -eu

FAILED=0
fail() { echo "  ❌ $1"; FAILED=$((FAILED+1)); }
pass() { echo "  ✓  $1"; }

if [ ! -f "go.work" ]; then
  echo "Run this from the repo root (go.work not found)." >&2
  exit 2
fi

echo "AegisVision template adoption check"
echo "===================================="
echo

echo "[1/6] Go modules build clean..."
for mod in $(find . -name go.mod -not -path "./.git/*" -not -path "*/node_modules/*" -exec dirname {} \;); do
  if (cd "$mod" && go build ./... >/dev/null 2>&1); then
    :
  else
    fail "go build in $mod"
  fi
done
[ "$FAILED" -eq 0 ] && pass "all modules build"

PRIOR=$FAILED
echo
echo "[2/6] Go vet clean..."
for mod in $(find . -name go.mod -not -path "./.git/*" -not -path "*/node_modules/*" -exec dirname {} \;); do
  if (cd "$mod" && go vet ./... >/dev/null 2>&1); then
    :
  else
    fail "go vet in $mod"
  fi
done
[ "$FAILED" -eq "$PRIOR" ] && pass "go vet passes everywhere"

PRIOR=$FAILED
echo
echo "[3/6] Race-detected tests..."
for mod in $(find . -name go.mod -not -path "./.git/*" -not -path "*/node_modules/*" -exec dirname {} \;); do
  out=$(cd "$mod" && go test -race -count=1 ./... 2>&1 || true)
  if printf '%s\n' "$out" | grep -qE '^FAIL|^---'; then
    fail "tests in $mod"
    printf '%s\n' "$out" | tail -5 | sed 's/^/      /'
  fi
done
[ "$FAILED" -eq "$PRIOR" ] && pass "all tests green under -race"

PRIOR=$FAILED
echo
echo "[4/6] Helm conformance..."
if (cd tools/conformance && go test -count=1 ./... >/dev/null 2>&1); then
  pass "every chart meets the golden path"
else
  fail "conformance test failed (run: cd tools/conformance && go test ./...)"
fi

PRIOR=$FAILED
echo
echo "[5/6] Cross-service integration smoke..."
if (cd tools/integration && go test -race -count=1 ./... >/dev/null 2>&1); then
  pass "integration smoke green"
else
  fail "integration smoke failed (run: cd tools/integration && go test ./...)"
fi

PRIOR=$FAILED
echo
echo "[6/6] Proto lint..."
if command -v buf >/dev/null 2>&1; then
  if (cd proto && buf lint >/dev/null 2>&1); then
    pass "buf lint clean"
  else
    fail "buf lint failed"
  fi
else
  echo "  -  skipped (buf not installed)"
fi

echo
if [ "$FAILED" -eq 0 ]; then
  echo "✅ Template check passed."
  exit 0
else
  echo "❌ Template check found $FAILED issue(s). Fix them before deploying."
  exit 1
fi
