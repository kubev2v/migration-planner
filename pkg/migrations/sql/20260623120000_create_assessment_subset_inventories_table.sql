-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS assessment_subset_inventories (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    name TEXT NOT NULL,
    snapshot_id INTEGER NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    v_center_id TEXT NOT NULL,
    vms_count INTEGER NOT NULL,
    inventory jsonb NOT NULL
);

-- Index on snapshot_id for filter performance (List...BySnapshotID) and CASCADE DELETE performance
CREATE INDEX IF NOT EXISTS idx_assessment_subset_inventories_snapshot_id ON assessment_subset_inventories(snapshot_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_assessment_subset_inventories_snapshot_id;
DROP TABLE IF EXISTS assessment_subset_inventories;
-- +goose StatementEnd
