FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/semantic-search/ ./services/semantic-search/
WORKDIR /src/services/semantic-search
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/semantic-search ./cmd/semantic-search
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/semantic-search /usr/local/bin/semantic-search
USER 65532:65532
EXPOSE 8290 8291
ENTRYPOINT ["/usr/local/bin/semantic-search"]
