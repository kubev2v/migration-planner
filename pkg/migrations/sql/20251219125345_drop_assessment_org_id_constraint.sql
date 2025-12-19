-- +goose Up
-- +goose StatementBegin
ALTER TABLE assessments DROP CONSTRAINT org_id_name;
ALTER TABLE assessments ADD CONSTRAINT user_name UNIQUE (username, name);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- +goose StatementEnd
