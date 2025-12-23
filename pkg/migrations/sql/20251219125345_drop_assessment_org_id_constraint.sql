-- +goose Up
-- +goose StatementBegin
ALTER TABLE assessments DROP CONSTRAINT org_id_name;
ALTER TABLE assessments ADD CONSTRAINT org_id_user_name UNIQUE (org_id, username, name);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- +goose StatementEnd
