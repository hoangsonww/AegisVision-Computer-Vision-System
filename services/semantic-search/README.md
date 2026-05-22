# semantic-search

> **Cross-tenant semantic search over events + clips.**

`semantic-search` indexes:

- Event payloads (via embeddings from `llm-gateway`).
- Clip captions (auto-generated; per-tenant opt-in).

It exposes a similarity-search API; the caller's tenant scope is
enforced (no cross-tenant leakage).

---

## API

```
POST /v1/search
{
  "query": "people loitering near the door at night",
  "tenant_id": "t-7",
  "k": 10,
  "filter": {"kind":"DWELL"}
}
```

---

## Why a separate service

Two reasons:

1. Event-service's primary job is the low-latency write+SSE path;
   semantic search is read-heavy and OLAP-shaped.
2. We may want to swap the underlying vector store independently
   (today pgvector, possibly Milvus later).

---

## See also

- [`../event-service/README.md`](../event-service/README.md).
- [`../../pkg/embeddings/README.md`](../../pkg/embeddings/README.md).
