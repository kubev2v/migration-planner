-- +goose Up
-- +goose StatementBegin
CREATE TABLE groups (
    id VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP,
    name TEXT NOT NULL,
    description TEXT,
    kind VARCHAR(50) NOT NULL,
    icon VARCHAR NOT NULL,
    company VARCHAR(200) NOT NULL,
    UNIQUE (company, name),
    parent_id VARCHAR(255) REFERENCES groups(id)
);

CREATE TABLE members (
    id VARCHAR(255) PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP,
    username VARCHAR(255) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL,
    group_id VARCHAR(255) NOT NULL REFERENCES groups(id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE members;
DROP TABLE groups;
-- +goose StatementEnd
