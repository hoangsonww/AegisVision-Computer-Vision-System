FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/gpu-scheduler/ ./services/gpu-scheduler/
WORKDIR /src/services/gpu-scheduler
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/gpu-scheduler ./cmd/gpu-scheduler
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gpu-scheduler /usr/local/bin/gpu-scheduler
USER 65532:65532
EXPOSE 8200 8201
ENTRYPOINT ["/usr/local/bin/gpu-scheduler"]
