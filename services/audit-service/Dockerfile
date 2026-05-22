FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/audit-service/ ./services/audit-service/
WORKDIR /src/services/audit-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/audit-service ./cmd/audit-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/audit-service /usr/local/bin/audit-service
USER 65532:65532
EXPOSE 8092 8093
ENTRYPOINT ["/usr/local/bin/audit-service"]
