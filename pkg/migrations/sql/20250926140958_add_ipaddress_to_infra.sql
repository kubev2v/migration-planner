-- +goose Up
-- +goose StatementBegin
ALTER TABLE image_infras ADD COLUMN ip_address VARCHAR(20);
ALTER TABLE image_infras ADD COLUMN subnet_mask VARCHAR(20);
ALTER TABLE image_infras ADD COLUMN default_gateway VARCHAR(20);
ALTER TABLE image_infras ADD COLUMN dns VARCHAR(20);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE image_infras DROP COLUMN ip_address;
ALTER TABLE image_infras DROP COLUMN subnet_mask;
ALTER TABLE image_infras DROP COLUMN default_gateway;
ALTER TABLE image_infras DROP COLUMN dns;
-- +goose StatementEnd
