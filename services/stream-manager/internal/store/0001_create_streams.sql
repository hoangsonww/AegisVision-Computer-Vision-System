CREATE TABLE IF NOT EXISTS streams (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    project_id      TEXT NOT NULL,
    camera_id       TEXT NOT NULL DEFAULT '',
    site_id         TEXT NOT NULL DEFAULT '',
    name            TEXT NOT NULL,
    protocol        INTEGER NOT NULL,
    url_redacted    TEXT NOT NULL,
    health          INTEGER NOT NULL,
    observed_fps    DOUBLE PRECISION NOT NULL DEFAULT 0,
    observed_kbps   BIGINT NOT NULL DEFAULT 0,
    version         BIGINT NOT NULL DEFAULT 1,
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL,
    last_frame_at   TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_streams_tenant ON streams(tenant_id);
CREATE INDEX IF NOT EXISTS idx_streams_site   ON streams(tenant_id, site_id);
