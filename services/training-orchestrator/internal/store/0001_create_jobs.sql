CREATE TABLE IF NOT EXISTS training_jobs (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    name          TEXT NOT NULL,
    base_model    TEXT NOT NULL,
    dataset_id    TEXT NOT NULL,
    dataset_ver   TEXT NOT NULL,
    state         TEXT NOT NULL DEFAULT 'pending', -- pending|running|succeeded|failed|cancelled
    epochs_done   INTEGER NOT NULL DEFAULT 0,
    epochs_total  INTEGER NOT NULL,
    metrics_json  TEXT NOT NULL DEFAULT '{}',
    artifact_uri  TEXT NOT NULL DEFAULT '',
    last_error    TEXT NOT NULL DEFAULT '',
    submitted_at  TIMESTAMP NOT NULL,
    finished_at   TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_jobs_tenant_state ON training_jobs(tenant_id, state);
