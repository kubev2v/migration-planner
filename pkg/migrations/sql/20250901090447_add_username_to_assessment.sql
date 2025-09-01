-- +goose Up
-- +goose StatementBegin
ALTER TABLE assessments ADD COLUMN username VARCHAR(255) NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE assessments DROP COLUMN username;
-- +goose StatementEnd
