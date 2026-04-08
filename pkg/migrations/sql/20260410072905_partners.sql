-- +goose Up
-- +goose StatementBegin
CREATE TYPE request_status AS ENUM ('pending','accepted','rejected');

CREATE TABLE partners_customers (
    id VARCHAR(255) NOT NULL,
    username VARCHAR(100) NOT NULL,
    partner_id VARCHAR(100) NOT NULL,
    request_status request_status NOT NULL DEFAULT 'pending',
    name VARCHAR(100) NOT NULL,
    contact_name VARCHAR(100) NOT NULL,
    contact_phone VARCHAR(100) NOT NULL,
    email VARCHAR(100) NOT NULL,
    location VARCHAR(100) NOT NULL,
    reason VARCHAR(255),
    PRIMARY KEY (id)
);

CREATE UNIQUE INDEX uq_partner_customer_active_username
    ON partners_customers (username)
    WHERE request_status IN ('pending', 'accepted');

CREATE INDEX idx_partners_customers_partner_id
    ON partners_customers (partner_id);


-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_partners_customers_partner_id;
DROP INDEX IF EXISTS uq_partner_customer_active_username;
DROP TABLE partners_customers;
DROP TYPE request_status;
-- +goose StatementEnd
