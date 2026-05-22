FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/drift-detection-service/ ./services/drift-detection-service/
WORKDIR /src/services/drift-detection-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/drift-detection-service ./cmd/drift-detection-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/drift-detection-service /usr/local/bin/drift-detection-service
USER 65532:65532
EXPOSE 8480 8481
ENTRYPOINT ["/usr/local/bin/drift-detection-service"]
