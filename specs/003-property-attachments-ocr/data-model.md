# Data Model: Property Attachments with Extracted Text

**Feature**: `003-property-attachments-ocr` | **Date**: 2026-08-31

Three forward-only migrations, `0016`–`0018`. All timestamps `timestamptz` in UTC, rendered
in Africa/Cairo. The Arabic normaliser is the one features 001 and 002 already use.

---

## 0016 — `attachment`

```sql
CREATE TYPE attachment_extract_state AS ENUM (
    'pending',        -- queued or running
    'done',           -- text found and stored
    'empty',          -- ran, found nothing; a normal outcome, not an error
    'not_eligible',   -- a type or a tool that cannot yield text
    'too_large',      -- beyond the processing bound
    'failed'          -- attempted and failed; retries exhausted
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
    nonce_prefix      bytea       NOT NULL,   -- 8 bytes; per-chunk nonce = prefix || counter
    chunk_size        integer     NOT NULL,

    is_sensitive      boolean     NOT NULL DEFAULT false,

    extract_state     attachment_extract_state NOT NULL DEFAULT 'pending',
    extract_attempts  integer     NOT NULL DEFAULT 0,
    extract_next_at   timestamptz NULL,
    extract_error     text        NULL,       -- a reason code, never file content

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
```

Points worth stating rather than inferring:

- **`ON DELETE RESTRICT` on `property_id`.** Feature 002 never destroys a property, so this
  is belt and braces — but it also means an attachment can never outlive its property as an
  orphan.
- **`extract_error` holds a reason code, never content.** Principle VII: a failure message
  that quotes the document defeats the encryption.
- **`attachment_attempts_bounded_chk` puts FR-020a in the schema.** Three automatic attempts
  is a rule, not a convention a retry loop might drift past.
- **`attachment_due_only_when_pending_chk`** makes "scheduled but not pending"
  unrepresentable, so the worker's due query cannot pick up a finished row.
- **The partial index on `extract_next_at`** is the worker's query. It stays small because
  it only holds pending rows.
- **`is_sensitive` has no CHECK forcing it false**, unlike feature 002's custom fields where
  only free text may be designated. Any attachment may be sensitive, because any document
  may contain an identifier.

### Sensitivity is one-way

FR-022c permits promotion and forbids demotion. This is enforced by a trigger rather than a
CHECK, because it constrains a transition rather than a state:

```sql
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
```

The application refuses demotion with an Arabic message before it reaches here. The trigger
is what makes the rule true rather than intended — including for a future code path nobody
has written yet.

### States

```text
                    ┌────────── done
   upload ──▶ pending ─────────  empty
                    │            not_eligible
                    │            too_large
                    └────────── failed ──┐
                         ▲               │
                         └───────────────┘
                     "try again" (FR-020b), or automatic
                     retry while attempts < 3

   Automatic: attempts 1, 2, 3 at 30 s, 2 min, 10 min. Then failed, and no longer due.
   Manual: allowed from failed and from empty. Refused when text has been corrected
           by hand (FR-020c) until the correction is cleared deliberately.
```

---

## 0017 — `attachment_text`

One row per attachment that has text. Ordinary text is stored readable and normalised;
sensitive text is stored only as ciphertext and has **no normalised column value at all**,
which is what makes it unsearchable structurally rather than by a `WHERE` clause
(research.md D-008).

```sql
CREATE TABLE attachment_text (
    attachment_id   uuid        PRIMARY KEY REFERENCES attachment(id) ON DELETE CASCADE,

    -- Ordinary: readable and searchable.
    body            text        NULL,
    body_normalized text        NULL,

    -- Sensitive: sealed with the single-shot envelope from feature 002 (research.md D-007).
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

CREATE INDEX attachment_text_search_idx ON attachment_text(attachment_id)
    WHERE body_normalized IS NOT NULL;
```

- **`ON DELETE CASCADE` here, uniquely in this project.** Everywhere else the rule is
  RESTRICT, because business records are never destroyed. Extracted text is not a business
  record — it is derived from one, and FR-009 requires it to go when its attachment goes.
  Cascade is the correct expression of "this has no life of its own".
- **`attachment_text_one_shape_chk` makes the two modes exclusive.** A row is readable text
  or sealed text, never both and never neither. It is also what guarantees a sensitive row
  has no `body_normalized` for a search to match.
- **`truncated` is surfaced to the user** (FR-021), so a phrase past the cut is explicable.
- **`corrected_at`/`corrected_by` move together**, and their presence is what FR-020c checks
  before permitting re-extraction.

### Promotion to sensitive

Promoting an attachment rewrites its text row in place, inside one transaction: read `body`,
seal it, write the cipher columns, and **null both `body` and `body_normalized`**. The row
must never be observable holding both. Because the search index is partial on
`body_normalized IS NOT NULL`, the row leaves the index in the same statement.

---

## 0018 — Privileges

```sql
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE attachment      TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE attachment_text TO pms_app;
```

`audit_log` is again not mentioned, keeping the `SELECT, INSERT` grant from feature 001, so
no code path in this feature can alter a record of who read what. `DELETE` is granted on
`attachment` because FR-008 removes attachments — this is the first feature in the project
where a business row is genuinely deleted rather than archived, and it is deliberate: an
attachment added in error should not be preserved forever, and the audit row records that
it existed and who removed it.

---

## Encryption layout

### File bodies — chunked (research.md D-001)

| Element | Where |
|---|---|
| KEK, 32 bytes | `PMS_KEK` environment variable. Never in the database, a log, or the repository. |
| Data key, 32 bytes | Fresh per attachment. Stored only as `wrapped_dek` + `wrap_nonce`. |
| Chunk nonce | `nonce_prefix` (8 random bytes, per file) ‖ chunk counter (4 bytes, big-endian). Unique per chunk under one key. |
| Chunk AAD | chunk index ‖ final-chunk flag. **This is what makes reordering and truncation detectable**; without it every chunk still verifies on its own. |
| Tag | Per chunk, verified on every read. A failure aborts the stream and returns nothing further. |

Stored file: a small header (wrapped key, prefix, chunk size, plaintext length) followed by
sealed chunks. The plaintext length is in the header so a truncated file is caught before a
byte is served, not only at the end.

### Extracted text — single-shot (research.md D-007)

Sensitive text only. Same `Seal`/`Open` pair feature 002 uses for a sensitive custom value,
with the ciphertext, nonce, wrapped key, and wrap nonce on the text row.

Decryption of either happens only inside a handler that has already resolved the caller's
role. No decrypted byte reaches a log, an error body, or an audit row.

---

## Entity relationships

```text
property  ──1:N──▶ attachment            (RESTRICT — an attachment never orphans)
account   ──created_by / updated_by / corrected_by──▶ attachment, attachment_text

attachment ──1:1──▶ attachment_text      (CASCADE — derived data, no life of its own)
attachment ──1:1──▶ one file in the store, named by the attachment id

audit_log  ──entity_type + entity_id──▶ attachment   (no FK, by design: the log records,
    it does not constrain, and it must outlive what it records)
```

`entity_type` gains no new enum value: an attachment audit row uses `property` as its entity
with the property's reference code, and names the attachment in `detail`. The question these
records answer is "what happened to this property", and an attachment that has been removed
still has to appear under the property it belonged to.
