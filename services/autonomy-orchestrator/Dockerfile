FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/autonomy-orchestrator/ ./services/autonomy-orchestrator/
WORKDIR /src/services/autonomy-orchestrator
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/autonomy-orchestrator ./cmd/autonomy-orchestrator
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/autonomy-orchestrator /usr/local/bin/autonomy-orchestrator
USER 65532:65532
EXPOSE 8510 8511
ENTRYPOINT ["/usr/local/bin/autonomy-orchestrator"]
