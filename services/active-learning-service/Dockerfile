FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.work go.work.sum* ./
COPY pkg/ ./pkg/
COPY services/active-learning-service/ ./services/active-learning-service/
WORKDIR /src/services/active-learning-service
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS='-trimpath' \
    go build -ldflags='-s -w' -o /out/active-learning-service ./cmd/active-learning-service
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/active-learning-service /usr/local/bin/active-learning-service
USER 65532:65532
EXPOSE 8440 8441
ENTRYPOINT ["/usr/local/bin/active-learning-service"]
