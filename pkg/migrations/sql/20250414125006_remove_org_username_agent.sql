-- +goose Up
-- +goose StatementBegin
ALTER TABLE agents DROP COLUMN IF EXISTS username;
ALTER TABLE agents DROP COLUMN IF EXISTS org_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- no statement because this migration is done to sync the old schema with the current one.
-- +goose StatementEnd
