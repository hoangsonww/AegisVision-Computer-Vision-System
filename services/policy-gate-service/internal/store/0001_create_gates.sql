-- policy gate records. One row per consequential agent action awaiting
-- human approval. Append-only by design: decisions update decision_*
-- columns, but the request itself is immutable.
CREATE TABLE IF NOT EXISTS policy_gates (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    tenant_id       TEXT NOT NULL,
    tool_name       TEXT NOT NULL,
    action_summary  TEXT NOT NULL,
    arguments_json  TEXT NOT NULL,
    blast_radius    TEXT NOT NULL DEFAULT 'medium',
    decision        TEXT NOT NULL DEFAULT 'pending',
    decided_by      TEXT NOT NULL DEFAULT '',
    decision_reason TEXT NOT NULL DEFAULT '',
    requested_at    TIMESTAMP NOT NULL,
    decided_at      TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_gates_tenant_decision ON policy_gates(tenant_id, decision);
CREATE INDEX IF NOT EXISTS idx_gates_session ON policy_gates(session_id);
