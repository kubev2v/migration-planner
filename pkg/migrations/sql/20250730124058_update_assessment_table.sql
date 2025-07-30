-- +goose Up
-- +goose StatementBegin
CREATE TABLE snapshots (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    inventory jsonb NOT NULL,
    assessment_id VARCHAR(255) NOT NULL REFERENCES assessments(id) ON DELETE CASCADE
);

ALTER TABLE assessments DROP COLUMN inventory;
ALTER TABLE assessments DROP CONSTRAINT assessments_source_id_fkey;
ALTER TABLE assessments ALTER COLUMN source_id DROP NOT NULL;
ALTER TABLE assessments ADD CONSTRAINT assessments_source_id_fkey FOREIGN KEY (source_id) references sources(id) ON DELETE SET NULL;
ALTER TABLE assessments ADD COLUMN org_id TEXT NOT NULL;
ALTER TABLE assessments ADD COLUMN source_type VARCHAR(100) NOT NULL;
ALTER TABLE assessments ADD CONSTRAINT org_id_name UNIQUE (org_id, name);
CREATE INDEX assessments_org_id_idx on assessments (org_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE snapshots;
DROP INDEX assessments_org_id_idx;
ALTER TABLE assessments DROP CONSTRAINT org_id_name;
ALTER TABLE assessments DROP COLUMN source;
ALTER TABLE assessments DROP COLUMN org_id;
ALTER TABLE assessments DROP CONSTRAINT assessments_source_id_fkey;
ALTER TABLE assessments ALTER COLUMN source_id SET NOT NULL;
ALTER TABLE assessments ADD CONSTRAINT assessments_source_id_fkey FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE;
ALTER TABLE assessments ADD COLUMN inventory jsonb NOT NULL;
-- +goose StatementEnd
