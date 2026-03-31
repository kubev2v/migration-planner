-- +goose Up
-- +goose StatementBegin
-- Drop the name_org_id unique constraint. This was possibly created by GORM AutoMigrate which resulted by it not being removed by 20260219150000_drop_source_org_id_constraint.sql
-- because that migration assumed the PostgreSQL auto-name (sources_name_org_id_key) rather than the GORM name (name_org_id).
ALTER TABLE sources DROP CONSTRAINT IF EXISTS name_org_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- +goose StatementEnd
