FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/dataplane-runner/ ./services/dataplane-runner/
WORKDIR /src/services/dataplane-runner
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/dataplane-runner ./cmd/dataplane-runner
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/dataplane-runner /usr/local/bin/dataplane-runner
USER 65532:65532
EXPOSE 9095
ENTRYPOINT ["/usr/local/bin/dataplane-runner"]
