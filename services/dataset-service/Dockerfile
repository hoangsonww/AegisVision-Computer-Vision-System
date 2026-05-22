FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/dataset-service/ ./services/dataset-service/
WORKDIR /src/services/dataset-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/dataset-service ./cmd/dataset-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/dataset-service /usr/local/bin/dataset-service
USER 65532:65532
EXPOSE 8250 8251
ENTRYPOINT ["/usr/local/bin/dataset-service"]
