FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/auth-proxy/ ./services/auth-proxy/
WORKDIR /src/services/auth-proxy
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/auth-proxy ./cmd/auth-proxy
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/auth-proxy /usr/local/bin/auth-proxy
USER 65532:65532
EXPOSE 8310 8311
ENTRYPOINT ["/usr/local/bin/auth-proxy"]
