-- +goose Up
-- +goose StatementBegin
CREATE SEQUENCE IF NOT EXISTS outbox_events_id_seq START 1;

CREATE TABLE IF NOT EXISTS outbox_events (
    id INTEGER PRIMARY KEY DEFAULT nextval('outbox_events_id_seq'),
    event_type VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS outbox_events;
DROP SEQUENCE IF EXISTS outbox_events_id_seq;
-- +goose StatementEnd
