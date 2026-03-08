-- +goose Up
-- +goose StatementBegin
ALTER TABLE image_infras ADD COLUMN agent_version TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
-- Backfill existing rows with baseline version for OVAs downloaded before version tracking
UPDATE image_infras
SET agent_version = 'v0.5.0'
WHERE agent_version IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE image_infras DROP COLUMN agent_version;
-- +goose StatementEnd
