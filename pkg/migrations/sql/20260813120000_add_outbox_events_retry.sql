-- +goose Up
-- +goose StatementBegin
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_outbox_events_next_retry_at ON outbox_events (next_retry_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_outbox_events_next_retry_at;
ALTER TABLE outbox_events DROP COLUMN IF EXISTS next_retry_at;
ALTER TABLE outbox_events DROP COLUMN IF EXISTS retry_count;
-- +goose StatementEnd
