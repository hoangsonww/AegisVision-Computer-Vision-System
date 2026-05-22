-- Per-tenant media references. Pixels live in object storage; we store the
-- URI + metadata + retention. Per the operational safeguard: cascading deletes and
-- copy-on-promote into training datasets are enforced by the application
-- layer, not the database.
CREATE TABLE IF NOT EXISTS media (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    stream_id     TEXT NOT NULL DEFAULT '',
    kind          TEXT NOT NULL,            -- clip | thumbnail | frame
    artifact_uri  TEXT NOT NULL,            -- s3://bucket/key
    mime_type     TEXT NOT NULL,
    bytes         BIGINT NOT NULL DEFAULT 0,
    width         INTEGER NOT NULL DEFAULT 0,
    height        INTEGER NOT NULL DEFAULT 0,
    duration_ms   BIGINT NOT NULL DEFAULT 0,
    retention_at  TIMESTAMP,                -- TTL drop date
    promoted      INTEGER NOT NULL DEFAULT 0,  -- copy-on-promote sentinel
    created_at    TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_media_tenant_stream ON media(tenant_id, stream_id);
CREATE INDEX IF NOT EXISTS idx_media_retention ON media(retention_at);
