-- +goose Up
-- +goose StatementBegin

-- Privileges for the attachment tables. DELETE is granted on attachment
-- because FR-008 removes attachments — this is the first feature in the
-- project where a business row is genuinely deleted rather than archived.
-- audit_log is again NOT mentioned, keeping the SELECT, INSERT grant from
-- feature 001, so no code path in this feature can alter a record of who
-- read or wrote what.

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE attachment      TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE attachment_text TO pms_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE attachment_text FROM pms_app;
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE attachment      FROM pms_app;

-- +goose StatementEnd
