# tools/proto

Helpers for regenerating the Buf-managed protobuf bindings under `proto/gen/`.

The canonical entry point is `task proto` (defined in the repo root
`Taskfile.yml`), which runs:

```sh
buf generate
```

inside `proto/`. The scripts in this directory are a thin wrapper that
also performs:

1. Toolchain check — verifies `buf`, `protoc-gen-go`, `protoc-gen-go-grpc`
   are installed (or installs them).
2. Lint + breaking-change checks against the previous commit.
3. Re-formats generated code with `gofmt`.

## Usage

```sh
./tools/proto/regen.sh         # check tools + buf generate
./tools/proto/regen.sh check   # lint + breaking only (no generate)
./tools/proto/install-tools.sh # install buf + protoc-gen-go locally
```

These are convenience entry points for human operators and CI. They are
deliberately small — the buf toolchain itself is the source of truth.
Per ADR-0007 (Protobuf everywhere, Buf-managed), `/proto` is the strictest
CODEOWNERS surface in the repo.
