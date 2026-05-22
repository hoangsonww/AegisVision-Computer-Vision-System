# notification-service

> **Webhooks, email, Slack.** Replay-safe idempotency.

`notification-service` consumes `events.v1` (and other selected
subjects) and delivers tenant-configured notifications: webhooks,
email, Slack channels, MS Teams.

```mermaid
flowchart LR
    EVENTS[(events.v1)] --> NS[notification-service]
    NS --> WEBHOOK[Webhook (HTTPS)]
    NS --> EMAIL[Email (SES)]
    NS --> SLACK[Slack]
```

---

## Idempotency

Every notification carries an idempotency key derived from
`(tenant_id, event_id, channel_id)`. The receiver dedupes on this
key. Replays from NATS JetStream are safe.

---

## Failure handling

- 5xx from webhook → retry with exponential backoff (1m, 5m, 15m, 1h, 6h, 24h).
- 4xx from webhook → DLQ (with `notification.failed.v1` published).
- Email bounce → mark channel quarantined; require operator action.

The chaos drill `deploy/chaos/webhook-receiver-5xx.yaml` injects 5xx
and asserts that retries happen + the DLQ does not overflow.

---

## See also

- [`../event-service/README.md`](../event-service/README.md).
