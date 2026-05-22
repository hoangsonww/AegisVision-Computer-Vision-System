FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/slo-watchdog/ ./services/slo-watchdog/
WORKDIR /src/services/slo-watchdog
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/slo-watchdog ./cmd/slo-watchdog
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/slo-watchdog /usr/local/bin/slo-watchdog
USER 65532:65532
EXPOSE 8490 8491
ENTRYPOINT ["/usr/local/bin/slo-watchdog"]
