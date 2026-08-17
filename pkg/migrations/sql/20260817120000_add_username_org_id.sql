-- +goose Up
-- +goose StatementBegin
-- members.org_id: the partner member's console.redhat.com org.
-- Default '0' is a sentinel value for existing rows (invalid org_id to be backfilled).
-- New members will get real org_id from IAM service on creation.
ALTER TABLE members ADD COLUMN IF NOT EXISTS org_id VARCHAR(255) NOT NULL DEFAULT '0';
-- partners_customers.username_org_id: the org_id belonging to the customer (username) on the request.
ALTER TABLE partners_customers ADD COLUMN IF NOT EXISTS username_org_id VARCHAR(255) NOT NULL DEFAULT '0';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE partners_customers DROP COLUMN IF EXISTS username_org_id;
ALTER TABLE members DROP COLUMN IF EXISTS org_id;
-- +goose StatementEnd
