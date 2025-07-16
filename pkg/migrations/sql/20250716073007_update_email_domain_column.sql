-- +goose Up
-- +goose StatementBegin
UPDATE sources SET email_domain = 'redhat.com' WHERE email_domain IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- No need to have a down migration here because this migration does not change the db schema.
-- +goose StatementEnd
