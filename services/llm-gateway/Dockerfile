FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/llm-gateway/ ./services/llm-gateway/
WORKDIR /src/services/llm-gateway
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/llm-gateway ./cmd/llm-gateway
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/llm-gateway /usr/local/bin/llm-gateway
USER 65532:65532
EXPOSE 8400 8401
ENTRYPOINT ["/usr/local/bin/llm-gateway"]
