# cost-accounting

> **Per-tenant internal cost accounting.** GPU-seconds, tokens, storage.

`cost-accounting` is the internal mirror of `metering-service`. Where
metering is what we *bill*, accounting is what we *spend*. They are
not the same number — margin lives between them.

Sources:

- `inference.completed.v1` (GPU-seconds).
- `llm.completed.v1` (input / output tokens, by model).
- `media.stored.v1` (object-store bytes).
- `event.persisted.v1` (ClickHouse row count, cheap signal).

---

## API

```
GET /v1/cost/by-tenant?from=&to=
GET /v1/cost/by-service?from=&to=
GET /v1/cost/margin?from=&to=
```

---

## See also

- [`../metering-service/README.md`](../metering-service/README.md).
- [`../llm-gateway/README.md`](../llm-gateway/README.md) — token accounting source.
