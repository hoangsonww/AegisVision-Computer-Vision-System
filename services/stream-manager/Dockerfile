FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/stream-manager/ ./services/stream-manager/
WORKDIR /src/services/stream-manager
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/stream-manager ./cmd/stream-manager
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/stream-manager /usr/local/bin/stream-manager
USER 65532:65532
EXPOSE 9092 9093
ENTRYPOINT ["/usr/local/bin/stream-manager"]
