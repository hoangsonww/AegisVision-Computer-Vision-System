FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/media-service/ ./services/media-service/
WORKDIR /src/services/media-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/media-service ./cmd/media-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/media-service /usr/local/bin/media-service
USER 65532:65532
EXPOSE 8280 8281
ENTRYPOINT ["/usr/local/bin/media-service"]
