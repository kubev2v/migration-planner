-- +goose Up
-- +goose StatementBegin
ALTER TABLE snapshots ADD COLUMN version SMALLINT NOT NULL DEFAULT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE snapshots DROP COLUMN version;
-- +goose StatementEnd
