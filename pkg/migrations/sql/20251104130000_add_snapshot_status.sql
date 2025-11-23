-- +goose Up
-- +goose StatementBegin
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'ready';
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS error TEXT;
ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP;
ALTER TABLE snapshots ALTER COLUMN inventory DROP NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE snapshots ALTER COLUMN inventory SET NOT NULL;
ALTER TABLE snapshots DROP COLUMN IF EXISTS updated_at;
ALTER TABLE snapshots DROP COLUMN IF EXISTS error;
ALTER TABLE snapshots DROP COLUMN IF EXISTS status;
-- +goose StatementEnd



