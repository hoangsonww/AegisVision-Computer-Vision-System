FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/agent-service/ ./services/agent-service/
WORKDIR /src/services/agent-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/agent-service ./cmd/agent-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/agent-service /usr/local/bin/agent-service
USER 65532:65532
EXPOSE 8420 8421
ENTRYPOINT ["/usr/local/bin/agent-service"]
