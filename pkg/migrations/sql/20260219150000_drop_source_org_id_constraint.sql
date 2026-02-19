-- +goose Up
-- +goose StatementBegin
-- Drop the original inline UNIQUE(name, org_id); PostgreSQL names it sources_name_org_id_key
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_name_org_id_key;
ALTER TABLE sources ADD CONSTRAINT sources_org_id_user_name UNIQUE (org_id, username, name);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- +goose StatementEnd
