# syntax=docker/dockerfile:1.7
#
# AegisVision umbrella image — every Go service binary baked into a single
# distroless container. Pick the binary at runtime:
#
#   docker run --rm ghcr.io/hoangsonww/aegisvision /usr/local/bin/api-gateway
#   docker run --rm ghcr.io/hoangsonww/aegisvision /usr/local/bin/pipeline-service
#   docker run --rm ghcr.io/hoangsonww/aegisvision /usr/local/bin/event-service
#
# Default entrypoint is api-gateway; override `command:` in Kubernetes/Compose
# to launch a different service.

# ---------- stage 1: build every service ----------
FROM golang:1.26-bookworm AS build
WORKDIR /src

ENV CGO_ENABLED=0 GOOS=linux GOFLAGS=-trimpath

COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/ ./services/
COPY tools/ ./tools/

RUN set -euo pipefail; \
    mkdir -p /out; \
    for cmd in services/*/cmd/*; do \
      svc="$(basename "$cmd")"; \
      svc_mod="$(dirname "$(dirname "$cmd")")"; \
      echo "==> building $svc"; \
      (cd "$svc_mod" && go build -ldflags='-s -w' -o "/out/$svc" "./cmd/$svc"); \
    done; \
    ls -l /out

# ---------- stage 2: distroless runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="AegisVision" \
      org.opencontainers.image.description="A distributed, GPU-native visual-intelligence OS" \
      org.opencontainers.image.source="https://github.com/hoangsonww/AegisVision-Computer-Vision-System" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=build /out/ /usr/local/bin/

USER 65532:65532
ENTRYPOINT ["/usr/local/bin/api-gateway"]
