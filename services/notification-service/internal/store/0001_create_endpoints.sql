CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    url         TEXT NOT NULL,
    secret      TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_endpoints_tenant_enabled
    ON webhook_endpoints(tenant_id, enabled);

CREATE TABLE IF NOT EXISTS webhook_dlq (
    id          TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL,
    tenant_id   TEXT NOT NULL,
    body        TEXT NOT NULL,
    attempts    INTEGER NOT NULL,
    last_error  TEXT NOT NULL,
    failed_at   TIMESTAMP NOT NULL
);
