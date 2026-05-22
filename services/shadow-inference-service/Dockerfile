FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/shadow-inference-service/ ./services/shadow-inference-service/
WORKDIR /src/services/shadow-inference-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/shadow-inference-service ./cmd/shadow-inference-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/shadow-inference-service /usr/local/bin/shadow-inference-service
USER 65532:65532
EXPOSE 8470 8471
ENTRYPOINT ["/usr/local/bin/shadow-inference-service"]
