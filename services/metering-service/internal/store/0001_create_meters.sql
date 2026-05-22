-- Per-tenant per-day counters. ClickHouse production uses a different DDL
-- (SummingMergeTree); the application picks the right schema at startup.
CREATE TABLE IF NOT EXISTS meters (
    tenant_id  TEXT NOT NULL,
    metric     TEXT NOT NULL,
    day        TEXT NOT NULL,      -- ISO 8601 date
    count      BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, metric, day)
);
CREATE INDEX IF NOT EXISTS idx_meters_tenant_day ON meters(tenant_id, day);
