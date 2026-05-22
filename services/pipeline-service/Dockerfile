# Distroless, non-root, scratch-thin runtime. The Helm chart adds readonly
# rootfs + seccomp + AppArmor on top.
FROM golang:1.26-bookworm AS build
WORKDIR /src

# Copy the whole workspace so go.work resolves cross-module deps. The build
# stays cacheable because /proto/gen/go and /pkg/platform change less than the
# service code itself.
COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/pipeline-service/ ./services/pipeline-service/

WORKDIR /src/services/pipeline-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/pipeline-service ./cmd/pipeline-service

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/pipeline-service /usr/local/bin/pipeline-service
USER 65532:65532
EXPOSE 9090 9091
ENTRYPOINT ["/usr/local/bin/pipeline-service"]
