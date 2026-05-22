-- Tenant master record. The tier drives quota + rate-limit defaults; the
-- per-tenant `crypto_key_id` is the handle to the Vault transit key whose
-- destruction crypto-shreds all tenant data (ADR-0014 / Matroid scar).
CREATE TABLE IF NOT EXISTS tenants (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    tier            TEXT NOT NULL DEFAULT 'standard', -- free|standard|enterprise
    crypto_key_id   TEXT NOT NULL,
    state           TEXT NOT NULL DEFAULT 'active',    -- active|suspended|deleted
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL,
    deleted_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_tenants_state ON tenants(state);

-- Quotas. The platform reads these on every mutating request via the
-- ratelimit middleware. Phase 3 deployment ships defaults per tier; a
-- per-tenant override row takes precedence.
CREATE TABLE IF NOT EXISTS quotas (
    tenant_id      TEXT NOT NULL,
    metric         TEXT NOT NULL,   -- pipelines | streams | events_per_minute | inferences_per_second | gpus | storage_bytes
    soft_limit     BIGINT NOT NULL,
    hard_limit     BIGINT NOT NULL,
    updated_at     TIMESTAMP NOT NULL,
    PRIMARY KEY (tenant_id, metric)
);

-- DSAR (data subject access request) tracking.
CREATE TABLE IF NOT EXISTS dsar_requests (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL,
    subject        TEXT NOT NULL,   -- subject identifier (email, ID, etc.)
    kind           TEXT NOT NULL,   -- access | erasure
    status         TEXT NOT NULL DEFAULT 'pending', -- pending | running | completed | failed
    requested_by   TEXT NOT NULL,
    requested_at   TIMESTAMP NOT NULL,
    completed_at   TIMESTAMP,
    artifact_uri   TEXT NOT NULL DEFAULT ''  -- pointer to the export bundle
);
CREATE INDEX IF NOT EXISTS idx_dsar_status ON dsar_requests(status);
