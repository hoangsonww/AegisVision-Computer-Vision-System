FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/realtime-hub/ ./services/realtime-hub/
WORKDIR /src/services/realtime-hub
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/realtime-hub ./cmd/realtime-hub
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/realtime-hub /usr/local/bin/realtime-hub
USER 65532:65532
EXPOSE 8220 8221
ENTRYPOINT ["/usr/local/bin/realtime-hub"]
