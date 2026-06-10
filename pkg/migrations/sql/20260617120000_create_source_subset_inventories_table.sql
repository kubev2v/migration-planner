-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS source_subset_inventories (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    deleted_at TIMESTAMP,
    name TEXT NOT NULL,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    v_center_id TEXT,
    vms_count INTEGER,
    inventory jsonb NOT NULL,
    update_type VARCHAR(10) DEFAULT 'auto' CHECK (update_type IN ('auto', 'manual'))
);

-- Index on source_id for filter performance (List...BySourceID) and CASCADE DELETE performance
CREATE INDEX IF NOT EXISTS idx_source_subset_inventories_source_id ON source_subset_inventories(source_id);

-- Add update_type to existing sources table
ALTER TABLE sources ADD COLUMN IF NOT EXISTS update_type VARCHAR(10) DEFAULT 'auto' CHECK (update_type IN ('auto', 'manual'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sources DROP COLUMN IF EXISTS update_type;
DROP INDEX IF EXISTS idx_source_subset_inventories_source_id;
DROP TABLE IF EXISTS source_subset_inventories;
-- +goose StatementEnd
