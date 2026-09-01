-- +goose Up
-- +goose StatementBegin

-- Searching an attachment by its description or original filename needs the
-- same Arabic matching the rest of the system uses — insensitive to case,
-- letter variants, diacritics and tatweel. That rule lives in Go
-- (identity.Canonical) and is deliberately not duplicated in SQL
-- (research.md D-007), so the canonical form is stored alongside the raw one
-- exactly as property name and code already are.
--
-- Existing rows are left NULL here rather than filled by an approximate SQL
-- rewrite: `admintool backfill-attachment-search` populates them through the
-- real normaliser. Search treats NULL as "not yet indexed" and falls back to a
-- raw case-insensitive match, so nothing is invisible in the meantime.

ALTER TABLE attachment ADD COLUMN IF NOT EXISTS description_normalized text NULL;
ALTER TABLE attachment ADD COLUMN IF NOT EXISTS filename_normalized    text NULL;

CREATE INDEX IF NOT EXISTS attachment_description_normalized_idx
    ON attachment(description_normalized);
CREATE INDEX IF NOT EXISTS attachment_filename_normalized_idx
    ON attachment(filename_normalized);

-- The has-attachments filter is an EXISTS on this column; the index on
-- (property_id, created_at) added in 0016 already serves it.

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS attachment_filename_normalized_idx;
DROP INDEX IF EXISTS attachment_description_normalized_idx;
ALTER TABLE attachment DROP COLUMN IF EXISTS filename_normalized;
ALTER TABLE attachment DROP COLUMN IF EXISTS description_normalized;
-- +goose StatementEnd
