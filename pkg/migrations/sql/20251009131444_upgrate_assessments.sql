-- +goose Up
-- +goose StatementBegin
DO $$
DECLARE s RECORD;
DECLARE assessment_id TEXT;
BEGIN
    FOR s IN SELECT id,name,username,org_id,inventory FROM sources where inventory IS NOT NULL 
        LOOP
            CASE WHEN EXISTS (SELECT 1 FROM assessments WHERE source_id = s.id) THEN
                RAISE NOTICE 'assessment exists..ignore';
            ELSE
                SELECT gen_random_uuid() INTO assessment_id;
                INSERT INTO assessments (id,name,source_id,org_id,source_type,username) values (assessment_id,s.name,s.id,s.org_id,'agent',s.username);
                INSERT INTO snapshots (inventory,assessment_id) VALUES (s.inventory, assessment_id);
            END CASE;
    END LOOP;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- +goose StatementEnd
