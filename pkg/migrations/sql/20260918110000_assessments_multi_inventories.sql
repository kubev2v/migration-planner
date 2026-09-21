-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS assessment_inventories (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    name TEXT NOT NULL,                         -- name of rvtools file or group if subset is true
    is_subset BOOLEAN NOT NULL DEFAULT FALSE,   -- true if it is a group's inventory
    vcenter_id TEXT NOT NULL,
    vms_count INTEGER NOT NULL,
    hosts_count INTEGER NOT NULL,
    networks_count INTEGER NOT NULL,
    datastores_count INTEGER NOT NULL,
    version SMALLINT NOT NULL DEFAULT 1,
    inventory jsonb NOT NULL,
    assessment_id VARCHAR(255) NOT NULL REFERENCES assessments(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_assessment_inventories_assessment_id
    ON assessment_inventories(assessment_id);

-- Backport from snapshots (full inventories, is_subset = false)
INSERT INTO assessment_inventories
    (id, created_at, name, is_subset, vcenter_id, vms_count,
     hosts_count, networks_count, datastores_count, version, inventory, assessment_id)
SELECT
    gen_random_uuid()::text,
    s.created_at,
    a.name,
    false,
    COALESCE(s.inventory->>'VCenterID', ''),
    COALESCE((s.inventory->'VCenter'->'VMs'->>'Total')::integer, 0),
    COALESCE((s.inventory->'VCenter'->'Infra'->>'TotalHosts')::integer, 0),
    COALESCE(jsonb_array_length(s.inventory->'VCenter'->'Infra'->'Networks'), 0),
    COALESCE(jsonb_array_length(s.inventory->'VCenter'->'Infra'->'Datastores'), 0),
    s.version,
    s.inventory,
    s.assessment_id
FROM snapshots s
JOIN assessments a ON a.id = s.assessment_id;

-- Backport from assessment_subset_inventories (group inventories, is_subset = true)
INSERT INTO assessment_inventories
    (id, created_at, name, is_subset, vcenter_id, vms_count,
     hosts_count, networks_count, datastores_count, version, inventory, assessment_id)
SELECT
    asi.id,
    asi.created_at,
    asi.name,
    true,
    asi.v_center_id,
    asi.vms_count,
    COALESCE((asi.inventory->'VCenter'->'Infra'->>'TotalHosts')::integer, 0),
    COALESCE(jsonb_array_length(asi.inventory->'VCenter'->'Infra'->'Networks'), 0),
    COALESCE(jsonb_array_length(asi.inventory->'VCenter'->'Infra'->'Datastores'), 0),
    s.version,
    asi.inventory,
    s.assessment_id
FROM assessment_subset_inventories asi
JOIN snapshots s ON s.id = asi.snapshot_id;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS assessment_inventories;
-- +goose StatementEnd
