CREATE TABLE IF NOT EXISTS autonomy_schedules (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    role               TEXT NOT NULL,
    trigger            TEXT NOT NULL,
    cron_expression    TEXT NOT NULL DEFAULT '',
    signal_subject     TEXT NOT NULL DEFAULT '',
    max_steps          INTEGER NOT NULL DEFAULT 8,
    max_autonomy_tier  INTEGER NOT NULL DEFAULT 2,
    paused             INTEGER NOT NULL DEFAULT 0,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_as_tenant ON autonomy_schedules(tenant_id);

CREATE TABLE IF NOT EXISTS autonomy_runs (
    id              TEXT PRIMARY KEY,
    schedule_id     TEXT NOT NULL,
    tenant_id       TEXT NOT NULL,
    role            TEXT NOT NULL,
    trigger         TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued',
    session_id      TEXT NOT NULL DEFAULT '',
    trigger_reason  TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMP NOT NULL,
    finished_at     TIMESTAMP,
    gate_id         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ar_schedule ON autonomy_runs(schedule_id, started_at);
CREATE INDEX IF NOT EXISTS idx_ar_tenant_status ON autonomy_runs(tenant_id, status);
