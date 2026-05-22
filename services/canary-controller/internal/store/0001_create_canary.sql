CREATE TABLE IF NOT EXISTS canary_plans (
    id                       TEXT PRIMARY KEY,
    tenant_id                TEXT NOT NULL,
    resource_urn             TEXT NOT NULL,
    baseline_urn             TEXT NOT NULL,
    candidate_urn            TEXT NOT NULL,
    ladder_json              TEXT NOT NULL,
    max_proportion_delta     REAL NOT NULL DEFAULT 0.01,
    max_latency_p95_ms_delta INTEGER NOT NULL DEFAULT 0,
    auto_promote             INTEGER NOT NULL DEFAULT 0,
    status                   TEXT NOT NULL DEFAULT 'draft',
    current_step             INTEGER NOT NULL DEFAULT 0,
    last_step_at             TIMESTAMP,
    failure_reason           TEXT NOT NULL DEFAULT '',
    created_at               TIMESTAMP NOT NULL,
    updated_at               TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cp_tenant ON canary_plans(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_cp_resource ON canary_plans(resource_urn);

CREATE TABLE IF NOT EXISTS canary_observations (
    plan_id        TEXT NOT NULL,
    variant        TEXT NOT NULL,
    window_start   TIMESTAMP NOT NULL,
    window_end     TIMESTAMP NOT NULL,
    successes      INTEGER NOT NULL DEFAULT 0,
    failures       INTEGER NOT NULL DEFAULT 0,
    latency_p50_ms INTEGER NOT NULL DEFAULT 0,
    latency_p95_ms INTEGER NOT NULL DEFAULT 0,
    latency_p99_ms INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (plan_id, variant, window_start)
);
CREATE INDEX IF NOT EXISTS idx_co_plan ON canary_observations(plan_id, window_end);
