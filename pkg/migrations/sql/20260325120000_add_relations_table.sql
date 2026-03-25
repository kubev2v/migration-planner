-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS relations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource VARCHAR NOT NULL,
    resource_id VARCHAR NOT NULL,
    relation VARCHAR NOT NULL,
    subject_namespace VARCHAR NOT NULL,
    subject_id VARCHAR NOT NULL,
    CONSTRAINT uq_resource_relation UNIQUE (resource, resource_id, relation, subject_namespace, subject_id)
);
CREATE INDEX idx_rel_subject_lookup ON relations (resource, subject_namespace, subject_id);
CREATE INDEX idx_rel_resource_subject ON relations (resource, resource_id, subject_namespace, subject_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS relations;
-- +goose StatementEnd
