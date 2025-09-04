-- +goose Up
-- +goose StatementBegin
ALTER TABLE assessments ADD COLUMN username VARCHAR(255);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE assessments DROP COLUMN username;
-- +goose StatementEnd
