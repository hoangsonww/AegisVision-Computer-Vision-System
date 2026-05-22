FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/inference-router/ ./services/inference-router/
WORKDIR /src/services/inference-router
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/inference-router ./cmd/inference-router
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/inference-router /usr/local/bin/inference-router
USER 65532:65532
EXPOSE 8094 8095
ENTRYPOINT ["/usr/local/bin/inference-router"]
