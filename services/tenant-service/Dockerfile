FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/tenant-service/ ./services/tenant-service/
WORKDIR /src/services/tenant-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/tenant-service ./cmd/tenant-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tenant-service /usr/local/bin/tenant-service
USER 65532:65532
EXPOSE 8300 8301
ENTRYPOINT ["/usr/local/bin/tenant-service"]
