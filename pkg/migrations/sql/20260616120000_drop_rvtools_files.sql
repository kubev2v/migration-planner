-- +goose Up
DROP TABLE IF EXISTS rvtools_files;

-- +goose Down
-- +goose StatementBegin
CREATE TABLE rvtools_files (
    id UUID PRIMARY KEY,
    data BYTEA NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT now()
);
-- +goose StatementEnd
