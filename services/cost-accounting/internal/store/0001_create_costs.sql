-- Per-tenant per-day cost lines. The collector ingests OpenCost/Kubecost
-- streams (CPU+memory+storage) and DCGM GPU-second exports, projects them
-- onto tenant+pipeline using K8s labels, and writes one row per
-- (tenant, day, line_item).
CREATE TABLE IF NOT EXISTS cost_lines (
    tenant_id     TEXT NOT NULL,
    pipeline_id   TEXT NOT NULL DEFAULT '',
    day           TEXT NOT NULL,
    line_item     TEXT NOT NULL,   -- cpu | memory | storage | gpu_seconds | inference_count | webhook_calls | ...
    quantity      REAL NOT NULL DEFAULT 0,
    unit_cost_usd REAL NOT NULL DEFAULT 0,
    total_usd     REAL NOT NULL DEFAULT 0,
    updated_at    TIMESTAMP NOT NULL,
    PRIMARY KEY (tenant_id, day, line_item, pipeline_id)
);
CREATE INDEX IF NOT EXISTS idx_cost_tenant_day ON cost_lines(tenant_id, day);
