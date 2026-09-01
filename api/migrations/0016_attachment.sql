-- +goose Up
-- +goose StatementBegin

-- Feature 003-property-attachments-ocr — attachment metadata and state.
--
-- Every uploaded file is encrypted at rest; this table holds the metadata,
-- the wrapped data key, and the per-file nonce prefix. The body itself lives
-- in a separate store directory named by the row's id (research.md D-011,
-- data-model.md). Sensitivity is one-way: it may be promoted but not demoted,
-- and that rule is enforced by the trigger below rather than by the
-- application, so a future code path cannot violate it (FR-022c).

CREATE TYPE attachment_extract_state AS ENUM (
    'pending',
    'done',
    'empty',
    'not_eligible',
    'too_large',
    'failed'
);

CREATE TABLE attachment (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    property_id       uuid        NOT NULL REFERENCES property(id) ON DELETE RESTRICT,

    description       text        NOT NULL,
    original_filename text        NOT NULL,
    content_type      text        NOT NULL,
    byte_size         bigint      NOT NULL,

    -- Envelope material for the body. The body itself is a file in the store,
    -- named by this row's id; see research.md D-011.
    wrapped_dek       bytea       NOT NULL,
    wrap_nonce        bytea       NOT NULL,
    nonce_prefix      bytea       NOT NULL,
    chunk_size        integer     NOT NULL,

    is_sensitive      boolean     NOT NULL DEFAULT false,

    extract_state     attachment_extract_state NOT NULL DEFAULT 'pending',
    extract_attempts  integer     NOT NULL DEFAULT 0,
    extract_next_at   timestamptz NULL,
    extract_error     text        NULL,

    created_at        timestamptz NOT NULL DEFAULT now(),
    created_by        uuid        NOT NULL REFERENCES account(id) ON DELETE RESTRICT,
    updated_at        timestamptz NOT NULL DEFAULT now(),
    updated_by        uuid        NOT NULL REFERENCES account(id) ON DELETE RESTRICT,

    CONSTRAINT attachment_description_len_chk
        CHECK (char_length(description) BETWEEN 2 AND 200),
    CONSTRAINT attachment_size_positive_chk CHECK (byte_size > 0),
    CONSTRAINT attachment_attempts_bounded_chk
        CHECK (extract_attempts BETWEEN 0 AND 3),
    CONSTRAINT attachment_due_only_when_pending_chk
        CHECK (extract_next_at IS NULL OR extract_state = 'pending')
);

CREATE INDEX attachment_property_idx ON attachment(property_id, created_at DESC);
CREATE INDEX attachment_due_idx ON attachment(extract_next_at)
    WHERE extract_state = 'pending';

-- Sensitivity is one-way. The application refuses demotion in Arabic before
-- the row reaches the database; the trigger is what makes the rule true
-- rather than intended, including for a future code path nobody has written
-- yet (research.md, FR-022c). Demoting a sensitive attachment would publish
-- text that was accepted as confidential, which Principle VII forbids.
CREATE FUNCTION attachment_sensitivity_is_one_way() RETURNS trigger AS $$
BEGIN
    IF OLD.is_sensitive AND NOT NEW.is_sensitive THEN
        RAISE EXCEPTION 'an attachment cannot be made non-sensitive';
    END IF;
    RETURN NEW;
END$$ LANGUAGE plpgsql;

CREATE TRIGGER attachment_sensitivity_one_way
    BEFORE UPDATE ON attachment
    FOR EACH ROW EXECUTE FUNCTION attachment_sensitivity_is_one_way();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS attachment_sensitivity_one_way ON attachment;
DROP FUNCTION IF EXISTS attachment_sensitivity_is_one_way();
DROP INDEX IF EXISTS attachment_due_idx;
DROP INDEX IF EXISTS attachment_property_idx;
DROP TABLE IF EXISTS attachment;
DROP TYPE IF EXISTS attachment_extract_state;

-- +goose StatementEnd
