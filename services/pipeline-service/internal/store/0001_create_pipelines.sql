-- 0001 — initial schema for pipeline-service.
--
-- The DAG itself is stored as compact JSON; pipeline-service does not crack
-- it open. The downstream compiler (pipeline-compiler in Phase 2) is the
-- only consumer that needs schema knowledge.
CREATE TABLE IF NOT EXISTS pipelines (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    project_id       TEXT NOT NULL,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    labels_json      TEXT NOT NULL DEFAULT '[]',
    status           INTEGER NOT NULL,
    current_revision BIGINT NOT NULL DEFAULT 1,
    dag_json         TEXT NOT NULL DEFAULT '',
    version          BIGINT NOT NULL DEFAULT 1,
    created_at       TIMESTAMP NOT NULL,
    updated_at       TIMESTAMP NOT NULL,
    created_by       TEXT NOT NULL DEFAULT '',
    updated_by       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pipelines_tenant_project
    ON pipelines(tenant_id, project_id);
CREATE INDEX IF NOT EXISTS idx_pipelines_tenant_status
    ON pipelines(tenant_id, status);
