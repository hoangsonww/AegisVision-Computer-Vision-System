CREATE TABLE IF NOT EXISTS slo_targets (
    id                     TEXT PRIMARY KEY,
    tenant_id              TEXT NOT NULL,
    name                   TEXT NOT NULL,
    promql                 TEXT NOT NULL,
    target                 REAL NOT NULL DEFAULT 0.999,
    rolling_window_seconds INTEGER NOT NULL DEFAULT 2419200,
    paused                 INTEGER NOT NULL DEFAULT 0,
    created_at             TIMESTAMP NOT NULL,
    updated_at             TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_st_tenant ON slo_targets(tenant_id);

CREATE TABLE IF NOT EXISTS slo_signals (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    target_id    TEXT NOT NULL,
    burn_rate_1h REAL NOT NULL DEFAULT 0,
    burn_rate_6h REAL NOT NULL DEFAULT 0,
    burn_rate_24h REAL NOT NULL DEFAULT 0,
    severity     TEXT NOT NULL,
    emitted_at   TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ss_target ON slo_signals(target_id, emitted_at);
