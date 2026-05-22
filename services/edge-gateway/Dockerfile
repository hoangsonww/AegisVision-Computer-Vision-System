FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/edge-gateway/ ./services/edge-gateway/
WORKDIR /src/services/edge-gateway
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/edge-gateway ./cmd/edge-gateway
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/edge-gateway /usr/local/bin/edge-gateway
USER 65532:65532
EXPOSE 8320 8321
ENTRYPOINT ["/usr/local/bin/edge-gateway"]
