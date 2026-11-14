-- +goose Up
-- +goose StatementBegin
ALTER TABLE assessment_enhancement_data
    ADD COLUMN vsphere_port_mirroring_enabled BOOLEAN,
    ADD COLUMN vsphere_port_mirroring_details TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE assessment_enhancement_data
    DROP COLUMN vsphere_port_mirroring_enabled,
    DROP COLUMN vsphere_port_mirroring_details;
-- +goose StatementEnd
