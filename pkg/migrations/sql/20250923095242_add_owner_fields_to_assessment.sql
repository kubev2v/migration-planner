-- +goose Up
-- +goose StatementBegin
ALTER TABLE assessments ADD COLUMN owner_first_name VARCHAR(100);
ALTER TABLE assessments ADD COLUMN owner_last_name VARCHAR(100);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE assessments DROP COLUMN owner_first_name;
ALTER TABLE assessments DROP COLUMN owner_last_name;
-- +goose StatementEnd