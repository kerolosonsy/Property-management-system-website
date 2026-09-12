-- +goose Up
-- +goose StatementBegin

-- Principle VIII requires entity type and entity identifier on every audit
-- row. Feature 001's audit_log can only name an account. The new columns are
-- nullable so existing rows and queries stay valid.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'audit_entity') THEN
        CREATE TYPE audit_entity AS ENUM ('property', 'property_type', 'area', 'custom_field');
    END IF;
END$$;

ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS entity_type audit_entity NULL;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS entity_id   text         NULL;

CREATE INDEX IF NOT EXISTS audit_log_entity_idx ON audit_log(entity_type, entity_id);

-- pms_app privileges are unchanged: still SELECT, INSERT only on audit_log.

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS audit_log_entity_idx;
ALTER TABLE audit_log DROP COLUMN IF EXISTS entity_id;
ALTER TABLE audit_log DROP COLUMN IF EXISTS entity_type;
DROP TYPE  IF EXISTS audit_entity;

-- +goose StatementEnd
