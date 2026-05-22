# pkg/embeddings

> **Vector store + chunker.** pgvector-compatible.

Used by `knowledge-service` (RAG) and `semantic-search`.

---

## API

```go
type Store interface {
    Upsert(ctx, ns string, items []Item) error
    Query(ctx, ns string, vec []float32, k int, filter Filter) ([]Hit, error)
}

type Chunker interface {
    Chunk(doc Document) []Chunk
}
```

The chunker is markdown-aware: preserves heading context, caps chunks
at ~1200 tokens.

---

## Why pgvector

- One database, less ops.
- Tenant-scoped via `tenant_id` index.
- Backups inherit Postgres tooling (WAL-G).
- Migrating to Milvus / Pinecone later is a `Store` implementation
  swap.

---

## See also

- [`services/knowledge-service/README.md`](../../services/knowledge-service/README.md).
- [`services/semantic-search/README.md`](../../services/semantic-search/README.md).
