-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS sources (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    deleted_at TIMESTAMP,
    name TEXT NOT NULL,
    v_center_id TEXT,
    username TEXT,
    org_id TEXT NOT NULL,
    inventory jsonb,
    on_premises BOOLEAN,
    UNIQUE(name, org_id)
);

CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    deleted_at TIMESTAMP,
    status TEXT,
    status_info TEXT,
    cred_url TEXT,
    version TEXT,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS image_infras (
    id SERIAL,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    deleted_at TIMESTAMP,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    http_proxy_url TEXT,
    https_proxy_url TEXT,
    no_proxy_domains TEXT,
    certificate_chain TEXT,
    ssh_public_key TEXT,
    image_token_key TEXT,
    PRIMARY KEY (id, source_id)
);

CREATE TABLE IF NOT EXISTS keys (
    id TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    deleted_at TIMESTAMP,
    org_id TEXT,
    private_key TEXT NOT NULL,
    PRIMARY KEY (id, org_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS image_infras;
DROP TABLE IF EXISTS sources;
DROP TABLE IF EXISTS keys;
-- +goose StatementEnd
