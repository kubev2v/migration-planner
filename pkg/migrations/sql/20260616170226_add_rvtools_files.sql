-- +goose Up
-- +goose StatementBegin
CREATE TABLE rvtools_files (
    id UUID PRIMARY KEY,
    data BYTEA NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS rvtools_files;
-- +goose StatementEnd
