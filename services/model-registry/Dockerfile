FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/model-registry/ ./services/model-registry/
WORKDIR /src/services/model-registry
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/model-registry ./cmd/model-registry
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/model-registry /usr/local/bin/model-registry
USER 65532:65532
EXPOSE 9094 9097
ENTRYPOINT ["/usr/local/bin/model-registry"]
