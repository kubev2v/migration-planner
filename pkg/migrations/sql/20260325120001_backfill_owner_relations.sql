-- +goose Up
-- +goose StatementBegin
DELETE FROM assessments WHERE username is null and org_id = 'example';
INSERT INTO relations (resource, resource_id, relation, subject_namespace, subject_id)
SELECT 'assessment', a.id::text, 'owner', 'user', a.username
FROM assessments a
ON CONFLICT ON CONSTRAINT uq_resource_relation DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM relations
WHERE resource = 'assessment'
  AND relation = 'owner'
  AND subject_namespace = 'user';
-- +goose StatementEnd
