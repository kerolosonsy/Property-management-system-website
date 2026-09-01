-- +goose Up
-- +goose StatementBegin

-- Feature 003-property-attachments-ocr — text read out of one attachment.
--
-- One row per attachment that has text. An ordinary attachment's text is
-- stored readable and searchable; a sensitive attachment's text is sealed
-- with the single-shot envelope from feature 002 (research.md D-007) and has
-- NO body_normalized value at all — not an empty one, no value — which is
-- what makes the search exclusion structural rather than a WHERE clause
-- (research.md D-008, FR-022b, FR-029). The two modes are mutually exclusive
-- by the one_shape CHECK, so the row is never readable and encrypted at the
-- same time.
--
-- The ON DELETE CASCADE here is the only one in this project; everywhere
-- else the rule is RESTRICT. Extracted text is derived from one attachment
-- and has no life of its own (FR-009).

CREATE TABLE attachment_text (
    attachment_id   uuid        PRIMARY KEY REFERENCES attachment(id) ON DELETE CASCADE,

    -- Ordinary: readable and searchable.
    body            text        NULL,
    body_normalized text        NULL,

    -- Sensitive: sealed with the single-shot envelope from feature 002.
    cipher_body     bytea       NULL,
    cipher_nonce    bytea       NULL,
    wrapped_dek     bytea       NULL,
    wrap_nonce      bytea       NULL,

    truncated       boolean     NOT NULL DEFAULT false,
    corrected_at    timestamptz NULL,
    corrected_by    uuid        NULL REFERENCES account(id) ON DELETE RESTRICT,
    extracted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT attachment_text_one_shape_chk CHECK (
        (body IS NOT NULL AND body_normalized IS NOT NULL
             AND cipher_body IS NULL AND wrapped_dek IS NULL)
        OR
        (cipher_body IS NOT NULL AND cipher_nonce IS NOT NULL
             AND wrapped_dek IS NOT NULL AND wrap_nonce IS NOT NULL
             AND body IS NULL AND body_normalized IS NULL)
    ),
    CONSTRAINT attachment_text_corrected_pair_chk
        CHECK ((corrected_at IS NULL) = (corrected_by IS NULL))
);

-- The partial index is what search hits. It only holds rows whose normalised
-- column is non-null, which excludes the sensitive path structurally: a
-- sensitive row's body_normalized is NULL by the one-shape CHECK above, so
-- it never enters the index. Even a query that forgets to filter by
-- sensitivity cannot return it (research.md D-008).
CREATE INDEX attachment_text_search_idx ON attachment_text(attachment_id)
    WHERE body_normalized IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS attachment_text_search_idx;
DROP TABLE IF EXISTS attachment_text;

-- +goose StatementEnd
