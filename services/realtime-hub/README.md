# realtime-hub

> **WebSocket fan-out hub for the console and integrations.**

`realtime-hub` complements `event-service`'s SSE feed. It exposes
WebSocket subscriptions for:

- Console: aggregated, throttled feeds across many streams.
- Integrations: per-tenant filtered topics.

The hub *consumes* from `event-service` (via SSE internally, or
directly from NATS in cluster mode) and *fans out* to many WebSocket
subscribers per pod.

---

## Why both SSE and WebSocket

- **SSE** is one-way, HTTP-friendly, trivial to retry with
  `Last-Event-Id`. Great for SDKs.
- **WebSocket** allows client-side subscribe/unsubscribe, and lower
  per-connection overhead at scale (thousands of clients per pod).

The console uses WebSocket for the live dashboard; SDKs use SSE for
durable streams.

---

## API

`GET /v1/realtime` (WebSocket upgrade)

Client → server messages:

```json
{"op":"subscribe","topic":"events.dwell.t-7","filter":{"stream_id":"stream-dock-1"}}
{"op":"unsubscribe","topic":"events.dwell.t-7"}
```

Server → client messages: JSON events.

---

## See also

- [`../event-service/README.md`](../event-service/README.md).
- [`../api-gateway/README.md`](../api-gateway/README.md).
