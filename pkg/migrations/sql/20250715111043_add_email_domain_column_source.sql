-- +goose Up
-- +goose StatementBegin
ALTER TABLE sources ADD COLUMN IF NOT EXISTS email_domain VARCHAR(255);

-- default all existing sources' org_ids to redhat.com
UPDATE sources SET email_domain = 'redhat.com' WHERE org_id IN ('11009103', '13872092', '19194072', '18692352', '19006254', '19009423', '19010322', '19012400') AND email_domain IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sources DROP COLUMN IF EXISTS email_domain;
-- +goose StatementEnd
