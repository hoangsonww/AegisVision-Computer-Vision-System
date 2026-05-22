CREATE TABLE IF NOT EXISTS datasets (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL,  -- images | clips | mixed
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_datasets_tenant ON datasets(tenant_id);

CREATE TABLE IF NOT EXISTS dataset_versions (
    id           TEXT PRIMARY KEY,
    dataset_id   TEXT NOT NULL,
    semver       TEXT NOT NULL,
    item_count   BIGINT NOT NULL DEFAULT 0,
    artifact_uri TEXT NOT NULL,  -- object-store URI for the manifest
    immutable    INTEGER NOT NULL DEFAULT 1,
    created_at   TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dataset_versions_dataset ON dataset_versions(dataset_id);
