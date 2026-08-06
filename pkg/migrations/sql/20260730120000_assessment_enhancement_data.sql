-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS assessment_enhancement_data (
    assessment_id VARCHAR(255) NOT NULL PRIMARY KEY
        REFERENCES assessments(id) ON DELETE CASCADE,

    -- Deployed Environment
    deployed_env_environment VARCHAR(50),

    -- VMware Version Counts
    vmware_ver_perpetual_licenses_count INTEGER,
    vmware_ver_cores_licensing_count    INTEGER,
    vmware_ver_environment_count        INTEGER,
    vmware_ver_other_nodes_count        INTEGER,

    -- Active Environments
    active_env_environments TEXT[],

    -- VMware Subscription
    vmware_sub_level TEXT[],

    -- vSphere Core
    vsphere_vm_encryption_enabled  BOOLEAN,
    vsphere_vm_encryption_policy   TEXT,
    vsphere_srm_enabled            BOOLEAN,

    -- NSX / Aria
    nsx_features             TEXT[],
    aria_ops_features        TEXT[],
    aria_automation_features TEXT[],
    aria_secure_features     TEXT[],

    -- Customer Details
    customer_physical_locations_count INTEGER,
    customer_target_hardware         TEXT,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS assessment_enhancement_data;
-- +goose StatementEnd
