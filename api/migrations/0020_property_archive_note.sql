-- +goose Up
-- +goose StatementBegin

-- Feature: items 2 and 5. An archive note (why the property was archived, in
-- Arabic, up to 500 characters) is recorded with the archive and shown on the
-- property detail while archived, and in the audit row's detail. Restore
-- clears it. The column is forward-only — never an edit of an applied
-- migration (Constitution IV).

ALTER TABLE property ADD COLUMN archive_note text NULL;

ALTER TABLE property ADD CONSTRAINT property_archive_note_len_chk
    CHECK (archive_note IS NULL OR char_length(archive_note) <= 500);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE property DROP CONSTRAINT IF EXISTS property_archive_note_len_chk;
ALTER TABLE property DROP COLUMN IF EXISTS archive_note;

-- +goose StatementEnd