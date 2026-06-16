-- +goose Up
-- +goose StatementBegin
ALTER TABLE assessment_cluster_sizing_inputs ADD COLUMN IF NOT EXISTS compact_mode BOOLEAN NOT NULL DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE assessment_cluster_sizing_inputs DROP COLUMN IF EXISTS compact_mode;
-- +goose StatementEnd
