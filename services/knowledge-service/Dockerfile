FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/knowledge-service/ ./services/knowledge-service/
WORKDIR /src/services/knowledge-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/knowledge-service ./cmd/knowledge-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/knowledge-service /usr/local/bin/knowledge-service
USER 65532:65532
EXPOSE 8430 8431
ENTRYPOINT ["/usr/local/bin/knowledge-service"]
