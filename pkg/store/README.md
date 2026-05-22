# pkg/store

> **SQL access + migrate runner.** Patroni Postgres (prod), SQLite
> (dev/test).

A thin wrapper around `database/sql` with:

- Connection pooling tuned for Patroni HA.
- Health check that survives failover.
- Migration runner (forward-only, signed migrations).
- SQLite fallback for dev so services don't need Postgres to compile.

---

## API

```go
db, err := store.Open(ctx, store.Config{
    DSN: os.Getenv("AEGIS_PG_DSN"),
    MaxOpen: 30,
})
defer db.Close()

if err := store.Migrate(ctx, db, "//go:embed migrations/*.sql"); err != nil {
    log.Fatal(err)
}
```

---

## Why per-service migrations

Each service owns its schema. Cross-service joins are forbidden
(ADR-0001 spirit). A service's `internal/store/migrations/` is the
source of truth for its tables.

---

## See also

- [`services/audit-service/README.md`](../../services/audit-service/README.md) — append-only patterns.
- [`services/pipeline-service/README.md`](../../services/pipeline-service/README.md) — canonical CRUD-with-revisions.
