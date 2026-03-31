-- +goose Up
-- +goose StatementBegin
-- Drop the GORM AutoMigrate-created index on sources(name, org_id).
-- The previous migration (20260331120000) attempted DROP CONSTRAINT IF EXISTS name_org_id but was a no-op
-- because GORM created it as a standalone index (CREATE UNIQUE INDEX), not a named constraint.
DROP INDEX IF EXISTS name_org_id;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- +goose StatementEnd
