-- +goose Up
-- +goose StatementBegin
-- One-time cleanup of relations left dangling by group deletions that happened
-- before DeleteGroup purged authz tuples. Every "org" entry in relations uses a
-- group's id (as resource_id for membership tuples, as subject_id for sharing
-- tuples), so an org id with no matching groups row is an orphan.
-- groups.id and relations.resource_id/subject_id are all VARCHAR, so no cast is needed.

-- Group as resource: org:<groupID>#member@user:<username>
DELETE FROM relations
WHERE resource = 'org'
  AND resource_id NOT IN (SELECT id FROM groups);

-- Group as subject: assessment:<id>#viewer@org:<groupID>
DELETE FROM relations
WHERE subject_namespace = 'org'
  AND subject_id NOT IN (SELECT id FROM groups);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Irreversible: deleted orphaned authz tuples cannot be reconstructed.
SELECT 1;
-- +goose StatementEnd
