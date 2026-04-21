-- +goose Up
-- +goose StatementBegin
ALTER TABLE partners_customers ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE partners_customers ALTER COLUMN partner_id TYPE VARCHAR(255);
DELETE FROM partners_customers WHERE partner_id NOT IN (SELECT id FROM groups);
ALTER TABLE partners_customers
    ADD CONSTRAINT fk_partners_customers_group
    FOREIGN KEY (partner_id) REFERENCES groups(id) ON DELETE CASCADE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE partners_customers DROP CONSTRAINT IF EXISTS fk_partners_customers_group;
ALTER TABLE partners_customers ALTER COLUMN partner_id TYPE VARCHAR(100);
ALTER TABLE partners_customers DROP COLUMN created_at;
-- +goose StatementEnd
