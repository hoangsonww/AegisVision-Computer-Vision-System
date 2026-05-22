FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/notification-service/ ./services/notification-service/
WORKDIR /src/services/notification-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/notification-service ./cmd/notification-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/notification-service /usr/local/bin/notification-service
USER 65532:65532
EXPOSE 8230 8231
ENTRYPOINT ["/usr/local/bin/notification-service"]
