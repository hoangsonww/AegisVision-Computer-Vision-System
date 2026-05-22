-- Agent sessions and per-step audit trail. Sessions are append-only after
-- their terminal state; steps are append-only by construction.
CREATE TABLE IF NOT EXISTS agent_sessions (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    role        TEXT NOT NULL,
    goal        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    step_count  INTEGER NOT NULL DEFAULT 0,
    max_steps   INTEGER NOT NULL DEFAULT 12,
    gate_id     TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sess_tenant ON agent_sessions(tenant_id, created_at);
CREATE INDEX IF NOT EXISTS idx_sess_gate ON agent_sessions(gate_id);

CREATE TABLE IF NOT EXISTS agent_steps (
    session_id   TEXT NOT NULL,
    idx          INTEGER NOT NULL,
    kind         TEXT NOT NULL,
    summary      TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '',
    gate_id      TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMP NOT NULL,
    PRIMARY KEY (session_id, idx)
);
