CREATE TABLE IF NOT EXISTS drift_policies (
    id                  TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    model_id            TEXT NOT NULL,
    method              TEXT NOT NULL DEFAULT 'js',
    warn_threshold      REAL NOT NULL DEFAULT 0.1,
    critical_threshold  REAL NOT NULL DEFAULT 0.3,
    window_seconds      INTEGER NOT NULL DEFAULT 900,
    min_samples         INTEGER NOT NULL DEFAULT 200
);
CREATE INDEX IF NOT EXISTS idx_dp_tenant_model ON drift_policies(tenant_id, model_id);

CREATE TABLE IF NOT EXISTS drift_references (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL,
    model_id          TEXT NOT NULL,
    classes_json      TEXT NOT NULL,
    samples           INTEGER NOT NULL DEFAULT 0,
    captured_at       TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dr_tenant_model ON drift_references(tenant_id, model_id);

CREATE TABLE IF NOT EXISTS drift_signals (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    model_id    TEXT NOT NULL,
    method      TEXT NOT NULL,
    divergence  REAL NOT NULL,
    severity    TEXT NOT NULL,
    classes_json TEXT NOT NULL,
    emitted_at  TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ds_tenant_emitted ON drift_signals(tenant_id, emitted_at);
