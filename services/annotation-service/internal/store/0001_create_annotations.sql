CREATE TABLE IF NOT EXISTS annotations (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    dataset_id  TEXT NOT NULL,
    item_id     TEXT NOT NULL,
    annotator   TEXT NOT NULL,
    label       TEXT NOT NULL,
    bbox_x      REAL NOT NULL,
    bbox_y      REAL NOT NULL,
    bbox_w      REAL NOT NULL,
    bbox_h      REAL NOT NULL,
    review_status TEXT NOT NULL DEFAULT 'pending', -- pending | approved | rejected
    reviewer    TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_annotations_dataset_item ON annotations(dataset_id, item_id);
CREATE INDEX IF NOT EXISTS idx_annotations_review ON annotations(review_status);
