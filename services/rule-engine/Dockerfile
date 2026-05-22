FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY proto/ ./proto/
COPY pkg/ ./pkg/
COPY services/rule-engine/ ./services/rule-engine/
WORKDIR /src/services/rule-engine
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/rule-engine ./cmd/rule-engine
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/rule-engine /usr/local/bin/rule-engine
USER 65532:65532
EXPOSE 8211
ENTRYPOINT ["/usr/local/bin/rule-engine"]
