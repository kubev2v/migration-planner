-- +goose Up
-- +goose StatementBegin
-- Data was copied to assessment_inventories by 20260918110000.
DROP TABLE assessment_subset_inventories;
DROP TABLE snapshots;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

CREATE TABLE snapshots (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    inventory jsonb NOT NULL,
    assessment_id VARCHAR(255) NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    version SMALLINT NOT NULL DEFAULT 1
);

-- Original numeric IDs are unavailable; regenerate them from full inventories.
INSERT INTO snapshots (created_at, inventory, assessment_id, version)
SELECT created_at, inventory, assessment_id, version
FROM assessment_inventories
WHERE NOT is_subset
ORDER BY created_at, id;

CREATE TABLE assessment_subset_inventories (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    name TEXT NOT NULL,
    snapshot_id INTEGER NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    v_center_id TEXT NOT NULL,
    vms_count INTEGER NOT NULL,
    inventory jsonb NOT NULL
);

CREATE INDEX idx_assessment_subset_inventories_snapshot_id
    ON assessment_subset_inventories(snapshot_id);

-- Reconstruct subset associations using the latest full inventory of the same version.
INSERT INTO assessment_subset_inventories
    (id, created_at, name, snapshot_id, v_center_id, vms_count, inventory)
SELECT ai.id, ai.created_at, ai.name, s.id, ai.vcenter_id, ai.vms_count, ai.inventory
FROM assessment_inventories ai
JOIN LATERAL (
    SELECT id FROM snapshots
    WHERE assessment_id = ai.assessment_id AND version = ai.version
    ORDER BY created_at DESC, id DESC
    LIMIT 1
) s ON true
WHERE ai.is_subset;
-- +goose StatementEnd
