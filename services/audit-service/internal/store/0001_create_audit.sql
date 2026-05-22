-- audit log table. Indexed for the queries the dashboard runs (per-tenant +
-- per-verb + time range). In production this table lives in ClickHouse; the
-- SQL shape is a small enough subset that Postgres serves the read-side too.
CREATE TABLE IF NOT EXISTS audit_records (
    id          TEXT PRIMARY KEY,
    at          TIMESTAMP NOT NULL,
    service     TEXT NOT NULL,
    tenant_id   TEXT NOT NULL DEFAULT '',
    request_id  TEXT NOT NULL DEFAULT '',
    method      TEXT NOT NULL,
    path        TEXT NOT NULL,
    status      INTEGER NOT NULL,
    outcome     TEXT NOT NULL,
    user_agent  TEXT NOT NULL DEFAULT '',
    remote_ip   TEXT NOT NULL DEFAULT '',
    verb        TEXT NOT NULL DEFAULT '',
    resource    TEXT NOT NULL DEFAULT '',
    raw_json    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_tenant_at ON audit_records(tenant_id, at);
CREATE INDEX IF NOT EXISTS idx_audit_outcome   ON audit_records(outcome);
