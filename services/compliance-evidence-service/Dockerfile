FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/compliance-evidence-service/ ./services/compliance-evidence-service/
WORKDIR /src/services/compliance-evidence-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/compliance-evidence-service ./cmd/compliance-evidence-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/compliance-evidence-service /usr/local/bin/compliance-evidence-service
USER 65532:65532
EXPOSE 8520 8521
ENTRYPOINT ["/usr/local/bin/compliance-evidence-service"]
