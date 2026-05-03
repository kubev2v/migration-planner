-- +goose Up
-- +goose StatementBegin
ALTER TABLE image_infras ADD COLUMN agent_token TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE image_infras DROP COLUMN agent_token;
-- +goose StatementEnd
