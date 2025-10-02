-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS snapshots (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    inventory jsonb NOT NULL,
    assessment_id VARCHAR(255) NOT NULL REFERENCES assessments(id) ON DELETE CASCADE
);

-- Drop column only if it exists
ALTER TABLE assessments DROP COLUMN IF EXISTS inventory;

-- Drop constraint only if it exists
DO $$ 
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'assessments_source_id_fkey') THEN
        ALTER TABLE assessments DROP CONSTRAINT assessments_source_id_fkey;
    END IF;
END $$;

-- Alter column (no IF NOT NULL syntax, but won't error if already nullable)
ALTER TABLE assessments ALTER COLUMN source_id DROP NOT NULL;

-- Add constraint (will error if exists, need to check first)
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'assessments_source_id_fkey') THEN
        ALTER TABLE assessments ADD CONSTRAINT assessments_source_id_fkey 
            FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE SET NULL;
    END IF;
END $$;

ALTER TABLE assessments ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL;
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS source_type VARCHAR(100) NOT NULL;

-- Add constraint only if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'org_id_name') THEN
        ALTER TABLE assessments ADD CONSTRAINT org_id_name UNIQUE (org_id, name);
    END IF;
END $$;

-- Create index only if it doesn't exist
CREATE INDEX IF NOT EXISTS assessments_org_id_idx ON assessments (org_id);

ALTER TABLE assessments ADD COLUMN IF NOT EXISTS username VARCHAR(255);
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS owner_first_name VARCHAR(100);
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS owner_last_name VARCHAR(100);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE assessments DROP COLUMN IF EXISTS owner_first_name;
ALTER TABLE assessments DROP COLUMN IF EXISTS owner_last_name;
ALTER TABLE assessments DROP COLUMN IF EXISTS username;
DROP TABLE IF EXISTS snapshots;
DROP INDEX IF EXISTS assessments_org_id_idx;

DO $$ 
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'org_id_name') THEN
        ALTER TABLE assessments DROP CONSTRAINT org_id_name;
    END IF;
END $$;

ALTER TABLE assessments DROP COLUMN IF EXISTS source_type;
ALTER TABLE assessments DROP COLUMN IF EXISTS org_id;

DO $$ 
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'assessments_source_id_fkey') THEN
        ALTER TABLE assessments DROP CONSTRAINT assessments_source_id_fkey;
    END IF;
END $$;

-- Note: Can't conditionally SET NOT NULL, this will error if column has nulls
ALTER TABLE assessments ALTER COLUMN source_id SET NOT NULL;

DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'assessments_source_id_fkey') THEN
        ALTER TABLE assessments ADD CONSTRAINT assessments_source_id_fkey 
            FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE;
    END IF;
END $$;

ALTER TABLE assessments ADD COLUMN IF NOT EXISTS inventory jsonb NOT NULL;
-- +goose StatementEnd