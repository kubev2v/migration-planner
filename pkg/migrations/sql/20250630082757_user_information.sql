-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    username VARCHAR(255) PRIMARY KEY,
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    organization VARCHAR(255)
);
-- for backwards compability, we add a new column that reference the username in users table.
ALTER TABLE sources ADD COLUMN user_id VARCHAR(255) REFERENCES users(username);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sources DROP COLUMN user_id;
DROP TABLE users;
-- +goose StatementEnd
