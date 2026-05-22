-- Schema for the events.v1 firehose. The ClickHouse variant uses a
-- MergeTree partitioned by tenant/day; the SQLite/Postgres variant uses a
-- vanilla table that's portable enough for tests. The application code
-- selects DDL via the driver.
--
-- We use the SQLite/Postgres dialect by default; ClickHouse migrations live
-- under 0001_clickhouse.sql and are applied conditionally by the migrator
-- (see sql_persistent.go).
CREATE TABLE IF NOT EXISTS events (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    project_id    TEXT NOT NULL DEFAULT '',
    pipeline_id   TEXT NOT NULL DEFAULT '',
    stream_id     TEXT NOT NULL,
    kind          INTEGER NOT NULL,
    severity      INTEGER NOT NULL,
    frame_seq     BIGINT NOT NULL DEFAULT 0,
    capture_time  TIMESTAMP NOT NULL,
    emitted_at    TIMESTAMP NOT NULL,
    summary       TEXT NOT NULL DEFAULT '',
    rule_id       TEXT NOT NULL DEFAULT '',
    zone_id       TEXT NOT NULL DEFAULT '',
    track_id      TEXT NOT NULL DEFAULT '',
    class_label   TEXT NOT NULL DEFAULT '',
    score         REAL NOT NULL DEFAULT 0,
    bbox_x        REAL NOT NULL DEFAULT 0,
    bbox_y        REAL NOT NULL DEFAULT 0,
    bbox_w        REAL NOT NULL DEFAULT 0,
    bbox_h        REAL NOT NULL DEFAULT 0,
    traceparent   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_events_tenant_emitted
    ON events(tenant_id, emitted_at);
CREATE INDEX IF NOT EXISTS idx_events_stream
    ON events(tenant_id, stream_id, emitted_at);
