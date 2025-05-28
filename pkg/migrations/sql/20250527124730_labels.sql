-- +goose Up
-- +goose StatementBegin
CREATE TABLE labels (
    key VARCHAR(100),
    value VARCHAR(100) NOT NULL,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    PRIMARY KEY (key,source_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE labels;
-- +goose StatementEnd
