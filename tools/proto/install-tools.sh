#!/usr/bin/env bash
# Install the Buf toolchain + protoc plugins used by `task proto`.
# Idempotent — safe to re-run.
set -euo pipefail

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH=x86_64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

BUF_VERSION="1.45.0"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
mkdir -p "$BIN_DIR"

ensure() {
  local name="$1"; local install_cmd="$2"
  if command -v "$name" >/dev/null 2>&1; then
    echo "✓ $name already installed at $(command -v "$name")"
    return
  fi
  echo "↻ installing $name…"
  eval "$install_cmd"
}

ensure buf "curl -sSL 'https://github.com/bufbuild/buf/releases/download/v${BUF_VERSION}/buf-$(uname -s)-${ARCH}' -o '$BIN_DIR/buf' && chmod +x '$BIN_DIR/buf'"
ensure protoc-gen-go      'go install google.golang.org/protobuf/cmd/protoc-gen-go@latest'
ensure protoc-gen-go-grpc 'go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest'

cat <<INFO

Tools installed to:
  buf:                $(command -v buf || echo "$BIN_DIR/buf")
  protoc-gen-go:      $(command -v protoc-gen-go || echo "(\$GOPATH/bin)")
  protoc-gen-go-grpc: $(command -v protoc-gen-go-grpc || echo "(\$GOPATH/bin)")

Ensure $BIN_DIR and \$GOPATH/bin are on your PATH.
INFO
