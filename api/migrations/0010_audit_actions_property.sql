-- +goose NO TRANSACTION

-- +goose Up

-- PostgreSQL refuses to USE a new enum value inside the same transaction that
-- added it. goose wraps each migration in one by default, so the addition and
-- the first INSERT that names any of these values must be in separate
-- transactions. See data-model.md 0010.

ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'property_created';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'property_modified';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'property_archived';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'property_restored';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'property_code_changed';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'lookup_created';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'lookup_renamed';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'lookup_removed';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'custom_field_created';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'custom_field_renamed';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'custom_field_removed';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'custom_field_choice_added';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'custom_field_choice_removed';

-- +goose Down

-- goose does not support dropping an enum value. A downgrade requires
-- recreating audit_action; out of scope for a forward-only migration chain.
SELECT 1;
