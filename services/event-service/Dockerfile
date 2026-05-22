FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/event-service/ ./services/event-service/
WORKDIR /src/services/event-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/event-service ./cmd/event-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/event-service /usr/local/bin/event-service
USER 65532:65532
EXPOSE 8090 8091
ENTRYPOINT ["/usr/local/bin/event-service"]
