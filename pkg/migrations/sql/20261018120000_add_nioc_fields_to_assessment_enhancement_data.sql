-- +goose Up
-- +goose StatementBegin
ALTER TABLE assessment_enhancement_data
    ADD COLUMN vsphere_nioc_enabled BOOLEAN,
    ADD COLUMN vsphere_nioc_details TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE assessment_enhancement_data
    DROP COLUMN vsphere_nioc_enabled,
    DROP COLUMN vsphere_nioc_details;
-- +goose StatementEnd
