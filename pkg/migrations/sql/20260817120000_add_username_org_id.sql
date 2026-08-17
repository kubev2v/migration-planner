-- +goose Up
-- +goose StatementBegin
-- Nullable: existing rows are backfilled later via the IAM lookup by username.
-- members.org_id: the partner member's console.redhat.com org.
ALTER TABLE members ADD COLUMN IF NOT EXISTS org_id VARCHAR(255);
-- partners_customers.username_org_id: the org_id belonging to the customer (username) on the request.
ALTER TABLE partners_customers ADD COLUMN IF NOT EXISTS username_org_id VARCHAR(255);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE partners_customers DROP COLUMN IF EXISTS username_org_id;
ALTER TABLE members DROP COLUMN IF EXISTS org_id;
-- +goose StatementEnd
