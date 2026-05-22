FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/annotation-service/ ./services/annotation-service/
WORKDIR /src/services/annotation-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/annotation-service ./cmd/annotation-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/annotation-service /usr/local/bin/annotation-service
USER 65532:65532
EXPOSE 8260 8261
ENTRYPOINT ["/usr/local/bin/annotation-service"]
