-- +goose Up
-- +goose StatementBegin

-- Feature: items 5. An audit row may carry before/after for fields stored
-- unencrypted at rest, and may name the record it reverses (Constitution VIII
-- as amended in v1.4.0). All columns nullable so existing rows stay valid.
-- The database remains append-only: the pms_app role keeps its SELECT, INSERT
-- grant on audit_log and gains no UPDATE/DELETE.

ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS before_value      jsonb NULL;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS after_value       jsonb NULL;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS reverses_audit_id bigint NULL REFERENCES audit_log(id) ON DELETE RESTRICT;

-- An undo of a property-modified row that was on a stale version is refused
-- with 409. The undo's write path goes through the normal property update,
-- which already enforces version + archived. The undo's audit row records
-- the version it expected, so a replay would still conflict cleanly.
--
-- No additional constraint is needed for the foreign key — RESTRICT is the
-- correct behaviour: deleting an audit row is not a thing anyone may do,
-- so reversing must reference rows that continue to exist.

CREATE INDEX IF NOT EXISTS audit_log_reverses_idx
    ON audit_log(reverses_audit_id)
    WHERE reverses_audit_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS audit_log_reverses_idx;
ALTER TABLE audit_log DROP COLUMN IF EXISTS reverses_audit_id;
ALTER TABLE audit_log DROP COLUMN IF EXISTS after_value;
ALTER TABLE audit_log DROP COLUMN IF EXISTS before_value;
-- +goose StatementEnd