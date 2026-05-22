FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/prefetch-service/ ./services/prefetch-service/
WORKDIR /src/services/prefetch-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/prefetch-service ./cmd/prefetch-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/prefetch-service /usr/local/bin/prefetch-service
USER 65532:65532
EXPOSE 8500 8501
ENTRYPOINT ["/usr/local/bin/prefetch-service"]
