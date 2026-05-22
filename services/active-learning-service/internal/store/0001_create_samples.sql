CREATE TABLE IF NOT EXISTS learning_policies (
    id                  TEXT PRIMARY KEY,
    tenant_id           TEXT NOT NULL,
    model_id            TEXT NOT NULL,
    strategy            TEXT NOT NULL,
    sample_rate         REAL NOT NULL DEFAULT 0.01,
    min_uncertainty     REAL NOT NULL DEFAULT 0.2,
    max_uncertainty     REAL NOT NULL DEFAULT 0.8,
    diversity_classes   TEXT NOT NULL DEFAULT '[]',
    daily_budget        INTEGER NOT NULL DEFAULT 0,
    snapshot_ttl_days   INTEGER NOT NULL DEFAULT 30
);
CREATE INDEX IF NOT EXISTS idx_lp_tenant ON learning_policies(tenant_id);

CREATE TABLE IF NOT EXISTS learning_samples (
    id                    TEXT PRIMARY KEY,
    tenant_id             TEXT NOT NULL,
    model_id              TEXT NOT NULL,
    policy_id             TEXT NOT NULL,
    snapshot_urn          TEXT NOT NULL,
    predicted_class       TEXT NOT NULL,
    predicted_confidence  REAL NOT NULL,
    uncertainty           REAL NOT NULL,
    status                TEXT NOT NULL DEFAULT 'queued',
    annotation_task_id    TEXT NOT NULL DEFAULT '',
    final_class           TEXT NOT NULL DEFAULT '',
    sampled_at            TIMESTAMP NOT NULL,
    labeled_at            TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ls_tenant_status ON learning_samples(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_ls_policy ON learning_samples(policy_id);
