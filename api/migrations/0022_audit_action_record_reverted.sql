-- +goose NO TRANSACTION
--
-- The undo system introduces the action `record_reverted` (Constitution VIII
-- as amended in v1.4.0). PostgreSQL refuses to USE a new enum value inside
-- the transaction that added it, so this migration mirrors the pattern from
-- 0010 and 0019.

-- +goose Up
-- +goose StatementBegin
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'record_reverted';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd