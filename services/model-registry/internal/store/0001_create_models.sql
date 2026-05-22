CREATE TABLE IF NOT EXISTS models (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    name        TEXT NOT NULL,
    kind        INTEGER NOT NULL,
    family      TEXT NOT NULL DEFAULT '',
    labels_json TEXT NOT NULL DEFAULT '[]',
    version     BIGINT NOT NULL DEFAULT 1,
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_models_tenant_kind ON models(tenant_id, kind);

CREATE TABLE IF NOT EXISTS model_versions (
    id                 TEXT PRIMARY KEY,
    model_id           TEXT NOT NULL,
    semver             TEXT NOT NULL,
    channel            INTEGER NOT NULL,
    artifact_uri       TEXT NOT NULL,
    signature          TEXT NOT NULL DEFAULT '',
    sbom_uri           TEXT NOT NULL DEFAULT '',
    fairness_eval_uri  TEXT NOT NULL DEFAULT '',
    cost_profiles_json TEXT NOT NULL DEFAULT '[]',
    version            BIGINT NOT NULL DEFAULT 1,
    created_at         TIMESTAMP NOT NULL,
    updated_at         TIMESTAMP NOT NULL,
    UNIQUE(model_id, semver)
);
CREATE INDEX IF NOT EXISTS idx_model_versions_model ON model_versions(model_id);
