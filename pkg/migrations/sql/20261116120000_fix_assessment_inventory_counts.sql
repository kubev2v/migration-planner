-- +goose Up
-- +goose StatementBegin

-- OMA-3197: The backfill in 20260918110000_assessments_multi_inventories.sql
-- read the inventory JSON using PascalCase keys ('VCenter', 'VMs', 'Total',
-- 'Infra', 'TotalHosts', 'Networks', 'Datastores', 'VCenterID'). The stored
-- inventory JSON actually uses camelCase/snake_case keys ('vcenter', 'vms',
-- 'total', 'infra', 'totalHosts', 'networks', 'datastores', 'vcenter_id').
-- Because JSONB key lookups in Postgres are case-sensitive, every lookup
-- returned NULL and COALESCE(..., 0) wrote 0, so all backfilled (older)
-- assessments show 0/0/0/0 in the list view even though the inventory blob is
-- intact. This migration recomputes the denormalized counts and vcenter_id
-- from the preserved inventory blob, mirroring NewAssessmentInventory() in
-- internal/store/model/assessment_inventory.go (v1 vs v2 shape detection).

UPDATE assessment_inventories ai
SET vms_count        = c.vms_count,
    hosts_count      = c.hosts_count,
    networks_count   = c.networks_count,
    datastores_count = c.datastores_count,
    vcenter_id       = c.vcenter_id
FROM (
    SELECT
        id,
        COALESCE((data->'vms'->>'total')::integer, 0)         AS vms_count,
        COALESCE((data->'infra'->>'totalHosts')::integer, 0)  AS hosts_count,
        CASE WHEN jsonb_typeof(data->'infra'->'networks') = 'array'
             THEN jsonb_array_length(data->'infra'->'networks')
             ELSE 0 END                                        AS networks_count,
        CASE WHEN jsonb_typeof(data->'infra'->'datastores') = 'array'
             THEN jsonb_array_length(data->'infra'->'datastores')
             ELSE 0 END                                        AS datastores_count,
        vcenter_id_new                                         AS vcenter_id
    FROM (
        SELECT
            id,
            -- v2 inventory nests the data under "vcenter"; v1 is the data itself.
            CASE
                WHEN inventory->'clusters' IS NOT NULL
                  OR inventory->'vcenter_id' IS NOT NULL
                THEN inventory->'vcenter'
                ELSE inventory
            END AS data,
            CASE
                WHEN inventory->'clusters' IS NOT NULL
                  OR inventory->'vcenter_id' IS NOT NULL
                THEN COALESCE(inventory->>'vcenter_id', '')
                ELSE COALESCE(inventory->'vcenter'->>'id', '')
            END AS vcenter_id_new
        FROM assessment_inventories
    ) shaped
) c
WHERE ai.id = c.id
  AND (ai.vms_count        IS DISTINCT FROM c.vms_count
    OR ai.hosts_count      IS DISTINCT FROM c.hosts_count
    OR ai.networks_count   IS DISTINCT FROM c.networks_count
    OR ai.datastores_count IS DISTINCT FROM c.datastores_count
    OR ai.vcenter_id       IS DISTINCT FROM c.vcenter_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- No-op: this migration repairs corrupted denormalized counts in place.
-- The original (incorrect) zero values cannot and should not be restored.
-- +goose StatementEnd
