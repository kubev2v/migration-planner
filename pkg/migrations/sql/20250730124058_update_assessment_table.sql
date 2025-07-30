-- +goose Up
-- +goose StatementBegin
ALTER TABLE assessments DROP COLUMN source_id;
ALTER TABLE assessments ADD COLUMN org_id TEXT NOT NULL;
CREATE INDEX assessments_org_id_idx on assessments (org_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE assessments ADD COLUMN source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE;
ALTER TABLE assessments DROP COLUMN org_id;
DROP INDEX assessments_org_id_idx;
-- +goose StatementEnd
