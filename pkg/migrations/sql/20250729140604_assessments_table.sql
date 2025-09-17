-- +goose Up
-- +goose StatementBegin
CREATE TABLE assessments (
    id VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP,
    name TEXT NOT NULL,
    inventory jsonb NOT NULL,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE assessments;
-- +goose StatementEnd
