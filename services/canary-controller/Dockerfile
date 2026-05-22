FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/canary-controller/ ./services/canary-controller/
WORKDIR /src/services/canary-controller
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/canary-controller ./cmd/canary-controller
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/canary-controller /usr/local/bin/canary-controller
USER 65532:65532
EXPOSE 8460 8461
ENTRYPOINT ["/usr/local/bin/canary-controller"]
