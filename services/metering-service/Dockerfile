FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/metering-service/ ./services/metering-service/
WORKDIR /src/services/metering-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/metering-service ./cmd/metering-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/metering-service /usr/local/bin/metering-service
USER 65532:65532
EXPOSE 8240 8241
ENTRYPOINT ["/usr/local/bin/metering-service"]
