FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/nlq-service/ ./services/nlq-service/
WORKDIR /src/services/nlq-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/nlq-service ./cmd/nlq-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nlq-service /usr/local/bin/nlq-service
USER 65532:65532
EXPOSE 8450 8451
ENTRYPOINT ["/usr/local/bin/nlq-service"]
