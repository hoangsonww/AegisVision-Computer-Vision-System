FROM golang:1.26-bookworm AS build
WORKDIR /src

COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/api-gateway/ ./services/api-gateway/

WORKDIR /src/services/api-gateway
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/api-gateway ./cmd/api-gateway

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api-gateway /usr/local/bin/api-gateway
USER 65532:65532
EXPOSE 8080 8081
ENTRYPOINT ["/usr/local/bin/api-gateway"]
