FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/policy-gate-service/ ./services/policy-gate-service/
WORKDIR /src/services/policy-gate-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/policy-gate-service ./cmd/policy-gate-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/policy-gate-service /usr/local/bin/policy-gate-service
USER 65532:65532
EXPOSE 8410 8411
ENTRYPOINT ["/usr/local/bin/policy-gate-service"]
