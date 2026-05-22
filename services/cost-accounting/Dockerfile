FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/cost-accounting/ ./services/cost-accounting/
WORKDIR /src/services/cost-accounting
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/cost-accounting ./cmd/cost-accounting
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/cost-accounting /usr/local/bin/cost-accounting
USER 65532:65532
EXPOSE 8330 8331
ENTRYPOINT ["/usr/local/bin/cost-accounting"]
