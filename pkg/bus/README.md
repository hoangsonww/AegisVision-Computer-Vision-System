# pkg/bus

> **Event-bus abstraction.** NATS + Kafka + in-process + DualBus.

The bus is the platform's nervous system. Every per-event signal
(detections, inference completions, gate resolutions, model
promotions) flows through here.

```go
type Publisher interface {
    Publish(ctx context.Context, msg Message) error
}
type Subscriber interface {
    Subscribe(ctx context.Context, subject string, handler Handler) (Subscription, error)
}
type Message struct {
    Subject string
    Headers map[string]string  // trace_id, tenant_id, idempotency_key
    Data    []byte
}
```

---

## Implementations

| Impl | When |
| --- | --- |
| `nats` | Production primary. JetStream. Low latency. |
| `kafka` | Production durable. Long retention, replay. |
| `inmemory` | Tests + walking skeleton (`AEGIS_EMBED_NATS=true` uses NATS embedded). |
| `dualbus` | Wraps NATS + Kafka. Publish to both; consume from one. |

---

## DualBus

```go
b := bus.NewDualBus(natsBus, kafkaBus)
b.Publish(ctx, msg)   // → publishes to both
```

Low-latency consumers (metering, drift) read from NATS. Long-tail
analytics reads from Kafka.

---

## Subject taxonomy

Subjects are versioned (`.v1`) and domain-scoped. The 17 well-known
subjects + 7 wildcard pairs are tested in
`tools/integration/integration_test.go`. The test asserts every
documented subject has both a producer and a consumer.

See [`ARCHITECTURE.md`](../../ARCHITECTURE.md#bus-architecture) for the
full catalogue.

---

## No frames on the bus

`pkg/bus` is generic, but **nobody puts frames on it** (ADR-0008).
Use claim-check URNs (`pkg/dataplane/claimcheck`).

---

## Anti-patterns

- **Don't invent new subjects without updating the integration test.**
- **Don't publish bytes that should be claim-checked.** Use URNs.
- **Don't publish without a `trace_id` header.** OTel context propagation depends on it.
