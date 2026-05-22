FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/training-orchestrator/ ./services/training-orchestrator/
WORKDIR /src/services/training-orchestrator
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/training-orchestrator ./cmd/training-orchestrator
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/training-orchestrator /usr/local/bin/training-orchestrator
USER 65532:65532
EXPOSE 8270 8271
ENTRYPOINT ["/usr/local/bin/training-orchestrator"]
