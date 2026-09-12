-- +goose NO TRANSACTION
--
-- The seven attachment actions feature 003 records. This MUST be its own
-- migration carrying NO TRANSACTION: PostgreSQL refuses to *use* a new enum
-- value inside the transaction that added it, and goose wraps each migration
-- in one by default. Adding the values and inserting rows that use them must
-- be separate migrations — the same rule migration 0010 follows for the
-- property actions.

-- +goose Up
-- +goose StatementBegin
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'attachment_added';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'attachment_described';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'attachment_sensitivity_raised';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'attachment_removed';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'attachment_read';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'attachment_text_corrected';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'attachment_reextract_requested';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- PostgreSQL cannot remove a value from an enum. Removing these would require
-- rebuilding the type and rewriting every audit row that references it, which
-- would mean altering an append-only log (Constitution VIII). The values are
-- left in place; they are inert when unused.
SELECT 1;
-- +goose StatementEnd
