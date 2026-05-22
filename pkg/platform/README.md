# pkg/platform

> **The golden path Go library that every service imports.**

This is the most-imported library in the repo. It carries a stronger
stability bar than a normal service: public function signatures don't
change without an ADR, removed exports go through a deprecation cycle
of at least one release, the error taxonomy is frozen, and middleware
order is documented and tested.

---

## Contents

| Package | Purpose |
| --- | --- |
| `config` | Env-driven config loader. Refuses unsafe defaults in `AEGIS_ENV=production`. |
| `logging` | `slog`-based structured logger with `tenant_id` / `request_id` / `trace_id` context propagation. |
| `otelinit` | OpenTelemetry bootstrap: tracer + meter + propagator. |
| `metrics` | RED helpers (rate / errors / duration). Histogram buckets standardised. |
| `problem` | RFC 9457 `application/problem+json` constructor + mapper. |
| `health` | Liveness + readiness probes. |
| `pagination` | HMAC-signed opaque cursors. |
| `idempotency` | Redis-backed idempotency cache (24h TTL). |
| `middleware` | HTTP middleware stack: auth, tenant, idempotency, RED, log, panic-recover, problem-encode. |
| `auth` | JWT verify against JWKS + tenant injection. |
| `shutdown` | Graceful shutdown coordinator. |
| `version` | Build metadata. |

---

## The middleware stack

Order matters. The standard order, in `middleware/Default()`:

```
1. PanicRecover         — catches all panics, returns 500 problem+json.
2. RequestID            — generates X-Request-Id if missing.
3. TraceContext         — OTel context propagation.
4. Logger               — slog with all context fields.
5. Auth                 — JWT verify (skipped on health/ready).
6. Tenant               — extracts tenant_id, sets on ctx.
7. Idempotency          — for mutating methods only.
8. RateLimit            — per-tenant.
9. RED                  — duration + result metrics.
10. (handler)
11. ProblemEncode       — error → problem+json.
```

Changing the order without testing breaks something. The order is
covered by `middleware_test.go`.

---

## Errors (RFC 9457)

```go
return problem.New(
    "https://aegisvision.example.com/problems/idempotency-conflict",
    "Idempotency-Key conflict",
    http.StatusConflict,
    problem.WithDetail("Different body under same key."),
    problem.WithInstance(r.URL.Path),
    problem.WithExtension("trace_id", trace.TraceIDFromContext(ctx)),
)
```

A small fixed type vocabulary lives in `problem/types.go`. Adding a
new type requires updating the catalogue.

---

## Config

```go
type Config struct {
    Env        Environment
    HTTPPort   int
    GRPCPort   int
    MetricsPort int
    LogLevel   slog.Level
    OTLPEndpoint string
    TraceSampleRate float64
    JWKSURL    string
    JWTIssuer  string
    CursorKey  []byte  // ≥ 32 bytes in prod
}

cfg, err := config.Load()       // reads env, validates
if cfg.Env == config.Production {
    cfg.MustHaveCursorKey()     // panics if absent or < 32 bytes
}
```

This is what makes production-shape services *refuse unsafe defaults*.

---

## Pagination

```go
cur := pagination.Encode(sortKey, sortValue, etag, ttl)
// → "v1.eyJrIjoiY3JlYXRlZF9hdC...."

ok, sortKey, sortValue, etag := pagination.Decode(cur)
if !ok { return problem.New(... 400) }
```

Tampered cursors fail HMAC verification → 400.

---

## Idempotency

Wraps a handler:

```go
mux.Handle("POST /v1/streams", middleware.Idempotency(handler))
```

The middleware reads `Idempotency-Key`, hashes the body, and stores
`(tenant_id, key, body_hash, response)`. Replays within 24h return the
cached response; same key + different body = 409.

---

## Metrics

```go
var streamCreates = metrics.NewCounter("aegis_streams_created_total", "tenant", "status")

streamCreates.With(prometheus.Labels{"tenant": tid, "status": "ok"}).Inc()
```

Pre-canned histograms for RED:

- `aegis_requests_total{service,method,status,tenant}`
- `aegis_request_duration_seconds{service,method}`

---

## Shutdown

```go
shutdown.Coordinate(ctx, 30*time.Second,
    httpServer.Shutdown,
    grpcServer.GracefulStop,
    natsConn.Drain,
    pgPool.Close,
)
```

Calls each in order, with a per-step deadline, and exits cleanly.

---

## Anti-patterns

- **Don't roll your own logger.** Use `logging`.
- **Don't put unbounded user input in a metric label.** Cardinality kills.
- **Don't return raw errors to the caller.** Always go through `problem`.
- **Don't use `fmt.Print*` for service code.** Use `slog`.
