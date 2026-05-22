# nlq-service

> **Natural-language query → structured query.**

`nlq-service` turns "show me all dwell events on dock-1 last Tuesday
night" into a structured query against `event-service` / ClickHouse.
Uses `llm-gateway` for the parse + slot-fill; never executes the
query — it returns the structured form to the caller for execution.

---

## Why return structured form, not execute

Two reasons:

1. **Tenant scoping is the caller's job.** The agent already has the
   tenant context; nlq-service should not duplicate that.
2. **Cite + audit.** The agent's audit trail wants the *structured*
   query, not free-form text. Determinism.

---

## API

```
POST /v1/nlq:parse
{
  "query": "dwell events on dock-1 last Tuesday night",
  "tenant_id": "t-7",
  "time_zone": "America/Los_Angeles"
}
→ {
  "filter": {
    "kind": "DWELL",
    "stream_id": "stream-dock-1",
    "from": "2026-05-13T20:00:00-08:00",
    "to":   "2026-05-14T04:00:00-08:00"
  }
}
```

---

## See also

- [`../llm-gateway/README.md`](../llm-gateway/README.md).
- [`../event-service/README.md`](../event-service/README.md).
- ADR-0020 (citation discipline propagates).
