# Data Model: Properties Register

**Feature**: `002-properties-crud` | **Date**: 2026-08-30

Seven forward-only migrations, `0009`–`0015`. `0015` is a correction shipped after review; see its section at the end. Every timestamp is `timestamptz` in UTC and is
rendered in Africa/Cairo by the client. No column in this feature holds money. The Arabic
normaliser referenced throughout is the one feature 001 applies to usernames: NFKC, strip
diacritics and tatweel, unify alef, ya, and ta-marbuta variants, case-fold.

---

## 0009 — `audit_log` gains an entity

Principle VIII requires entity type and entity identifier on every record. Feature 001's
table can only name an account. Both columns are nullable so every existing row and query
stays valid.

```sql
CREATE TYPE audit_entity AS ENUM ('property', 'property_type', 'area', 'custom_field');

ALTER TABLE audit_log ADD COLUMN entity_type audit_entity NULL;
ALTER TABLE audit_log ADD COLUMN entity_id   text         NULL;

CREATE INDEX audit_log_entity_idx ON audit_log(entity_type, entity_id);
```

`entity_id` is text, not uuid, because a property's business identity is its reference
code. A record reading `property / P-004` answers "what was archived" a year later;
a uuid does not.

**Constraint**: `pms_app` privileges are unchanged — still `SELECT, INSERT` only. Widening
the table does not widen the grant.

---

## 0010 — New audit actions

```sql
ALTER TYPE audit_action ADD VALUE 'property_created';
ALTER TYPE audit_action ADD VALUE 'property_modified';
ALTER TYPE audit_action ADD VALUE 'property_archived';
ALTER TYPE audit_action ADD VALUE 'property_restored';
ALTER TYPE audit_action ADD VALUE 'property_code_changed';
ALTER TYPE audit_action ADD VALUE 'lookup_created';
ALTER TYPE audit_action ADD VALUE 'lookup_renamed';
ALTER TYPE audit_action ADD VALUE 'lookup_removed';
ALTER TYPE audit_action ADD VALUE 'custom_field_created';
ALTER TYPE audit_action ADD VALUE 'custom_field_renamed';
ALTER TYPE audit_action ADD VALUE 'custom_field_removed';
ALTER TYPE audit_action ADD VALUE 'custom_field_choice_added';
ALTER TYPE audit_action ADD VALUE 'custom_field_choice_removed';
```

**This migration MUST carry `-- +goose NO TRANSACTION`.** PostgreSQL refuses to *use* a
new enum value inside the transaction that added it, and goose wraps each migration in one
by default. Adding the values and inserting rows that use them must be separate
migrations. Getting this wrong compiles, deploys, and fails on the first property created.

`lookup_created` / `lookup_renamed` / `lookup_removed` are shared between property types
and areas; `entity_type` distinguishes them, which is what that column is for.

---

## 0011 — The two lookup tables

Identical shape and identical rules, which is why the Angular panel is written once and
instantiated twice.

```sql
CREATE TABLE property_type (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    label           text        NOT NULL,
    label_normalized text       NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT property_type_label_len_chk CHECK (char_length(label) BETWEEN 1 AND 60)
);
CREATE UNIQUE INDEX property_type_label_normalized_key ON property_type(label_normalized);

CREATE TABLE area (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    label           text        NOT NULL,
    label_normalized text       NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT area_label_len_chk CHECK (char_length(label) BETWEEN 1 AND 60)
);
CREATE UNIQUE INDEX area_label_normalized_key ON area(label_normalized);
```

Seeded in the same migration, per FR-026: property types تجاري and سكني; areas taken from
the design source. `label_normalized` is computed by the application, never by a database
function — see research.md D-007 for why the rule lives in one language only.

**No regex CHECK on the label.** Feature 001 learned this the hard way: PostgreSQL accepts
a `\p{L}` class at `CREATE TABLE` and throws `invalid escape \ sequence` at the first
`INSERT`. Length is checked here; character rules are checked in Go.

---

## 0012 — `property`

```sql
CREATE SEQUENCE property_code_seq AS bigint START 1;

CREATE TABLE property (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    code             text        NOT NULL,
    code_normalized  text        NOT NULL,
    name             text        NOT NULL,
    name_normalized  text        NOT NULL,
    property_type_id uuid        NOT NULL REFERENCES property_type(id) ON DELETE RESTRICT,
    area_id          uuid        NOT NULL REFERENCES area(id)          ON DELETE RESTRICT,
    version          integer     NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL REFERENCES account(id) ON DELETE RESTRICT,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL REFERENCES account(id) ON DELETE RESTRICT,
    archived_at      timestamptz NULL,
    archived_by      uuid        NULL     REFERENCES account(id) ON DELETE RESTRICT,
    CONSTRAINT property_name_len_chk CHECK (char_length(name) BETWEEN 2 AND 120),
    CONSTRAINT property_code_len_chk CHECK (char_length(code) BETWEEN 1 AND 32),
    CONSTRAINT property_archived_pair_chk
        CHECK ((archived_at IS NULL) = (archived_by IS NULL))
);

CREATE UNIQUE INDEX property_code_normalized_key ON property(code_normalized);
CREATE INDEX property_name_normalized_idx ON property(name_normalized);
-- Both normalised columns are indexed because FR-009a filters each on its own.
CREATE INDEX property_active_name_idx ON property(name) WHERE archived_at IS NULL;
CREATE INDEX property_type_idx ON property(property_type_id);
CREATE INDEX property_area_idx ON property(area_id);
```

Points worth stating rather than inferring:

- **The unique index on `code_normalized` deliberately spans archived rows.** That single
  index is the whole of FR-017b and FR-022d: a code, once issued, is never available
  again, so restoring an archived property can never collide with something entered
  meanwhile.
- **There is no unique index on the name**, active or otherwise. FR-016a permits duplicate
  names on purpose; buildings named after the same saint are the norm here.
- **`archived_at` and `archived_by` move together**, enforced by the pair CHECK rather
  than by trust.
- **`version` is the concurrency token** (research.md D-005), incremented by every update
  including archive and restore.
- No foreign key uses `ON DELETE CASCADE` anywhere in this feature. Nothing is destroyed.

### State

Exactly two states, both directions permitted, no third state and no terminal state:

```text
                 archive (archived_at := now())
        active  ─────────────────────────────▶  archived
           ▲                                        │
           └────────────────────────────────────────┘
                 restore (archived_at := NULL)
```

An archived property is refused for modification (FR-022e); it must be restored first.
Reads of an archived property are permitted, which is what makes it "readable" per the
clarification.

---

## 0013 — Custom fields

```sql
CREATE TYPE custom_field_type AS ENUM ('text', 'dropdown', 'multiselect', 'checkbox');

CREATE TABLE custom_field (
    id               uuid              PRIMARY KEY DEFAULT gen_random_uuid(),
    label            text              NOT NULL,
    label_normalized text              NOT NULL,
    field_type       custom_field_type NOT NULL,
    is_sensitive     boolean           NOT NULL DEFAULT false,
    display_order    integer           NOT NULL DEFAULT 0,
    created_at       timestamptz       NOT NULL DEFAULT now(),
    updated_at       timestamptz       NOT NULL DEFAULT now(),
    CONSTRAINT custom_field_label_len_chk CHECK (char_length(label) BETWEEN 1 AND 60),
    CONSTRAINT custom_field_sensitive_text_only_chk
        CHECK (is_sensitive = false OR field_type = 'text')
);
CREATE UNIQUE INDEX custom_field_label_normalized_key ON custom_field(label_normalized);

CREATE TABLE custom_field_choice (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    custom_field_id  uuid        NOT NULL REFERENCES custom_field(id) ON DELETE RESTRICT,
    label            text        NOT NULL,
    label_normalized text        NOT NULL,
    display_order    integer     NOT NULL DEFAULT 0,
    CONSTRAINT custom_field_choice_label_len_chk CHECK (char_length(label) BETWEEN 1 AND 60)
);
CREATE UNIQUE INDEX custom_field_choice_label_key
    ON custom_field_choice(custom_field_id, label_normalized);
```

`custom_field_sensitive_text_only_chk` is Principle VII's first bound expressed as a
database constraint rather than as a code path. A dropdown cannot be marked sensitive,
because its value comes from a published list and encrypting it conceals nothing.

### Values

```sql
CREATE TABLE property_field_value (
    property_id     uuid    NOT NULL REFERENCES property(id)     ON DELETE RESTRICT,
    custom_field_id uuid    NOT NULL REFERENCES custom_field(id) ON DELETE RESTRICT,

    text_value      text    NULL,                                    -- text, non-sensitive
    bool_value      boolean NULL,                                    -- checkbox
    choice_id       uuid    NULL REFERENCES custom_field_choice(id) ON DELETE RESTRICT,

    cipher_value    bytea   NULL,                                    -- text, sensitive
    cipher_nonce    bytea   NULL,
    wrapped_dek     bytea   NULL,
    wrap_nonce      bytea   NULL,

    PRIMARY KEY (property_id, custom_field_id),
    CONSTRAINT property_field_value_one_shape_chk CHECK (
        num_nonnulls(text_value, bool_value, choice_id, cipher_value) = 1
    ),
    CONSTRAINT property_field_value_cipher_complete_chk CHECK (
        (cipher_value IS NULL AND cipher_nonce IS NULL
            AND wrapped_dek IS NULL AND wrap_nonce IS NULL)
        OR
        (cipher_value IS NOT NULL AND cipher_nonce IS NOT NULL
            AND wrapped_dek IS NOT NULL AND wrap_nonce IS NOT NULL)
    )
);

CREATE TABLE property_field_multi_value (
    property_id     uuid NOT NULL REFERENCES property(id)            ON DELETE RESTRICT,
    custom_field_id uuid NOT NULL REFERENCES custom_field(id)        ON DELETE RESTRICT,
    choice_id       uuid NOT NULL REFERENCES custom_field_choice(id) ON DELETE RESTRICT,
    PRIMARY KEY (property_id, custom_field_id, choice_id)
);
```

- **`ON DELETE RESTRICT` on every reference is FR-027g.** Removing a field, retyping it, or
  removing one of its choices is refused by the database while any value depends on it.
  The application counts the dependants first only so the Arabic message can say how many;
  the constraint is the guarantee (research.md D-009).
- **`property_field_value_one_shape_chk` means a row holds exactly one kind of value.** It
  is what stops a text value and a choice from coexisting after a botched update.
- **The four cipher columns are all-or-nothing.** A ciphertext without its nonce or its
  wrapped key is unreadable forever; the constraint makes that state unrepresentable.
- **`cipher_value` never holds a plaintext fallback.** There is no code path that writes a
  sensitive field's value to `text_value`.
- A property with no row for a field simply has no value for it — FR-027d makes every
  custom field optional, so absence is normal, not an error.

---

## 0014 — Privileges

```sql
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE property                   TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE property_type              TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE area                       TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE custom_field               TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE custom_field_choice        TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE property_field_value       TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE property_field_multi_value TO pms_app;
GRANT USAGE ON SEQUENCE property_code_seq TO pms_app;
```

`audit_log` is not mentioned, and that is the point: it keeps the `SELECT, INSERT` grant
feature 001 gave it, so no code path in this feature can update or delete a record even by
mistake. Feature 001's migration 0007 already narrowed `ALTER DEFAULT PRIVILEGES` so that
new tables do not silently acquire write rights; these grants are therefore explicit.

---

## Encryption at rest

Applies only to `property_field_value` rows whose `custom_field.is_sensitive` is true.

| Element | Where it lives |
|---|---|
| KEK, 32 bytes | `PMS_KEK` environment variable, base64. Never in the database, a log, an error body, or the repository. |
| DEK, 32 bytes | Generated fresh per value. Stored only as `wrapped_dek`, sealed under the KEK with `wrap_nonce`. |
| Ciphertext | `cipher_value`, AES-256-GCM under the DEK with `cipher_nonce`. |
| Authentication tag | Appended to the ciphertext by GCM; verified on every open. A failure returns an error and no content. |

Decryption happens only inside a handler that has already resolved the caller's role from
the session (FR-027s3). A decrypted value is never written to a log, an error message, or
an audit row — audit rows for these fields name the field, never its value. The scheme
above is FR-027s2 in full: per-value data key, wrapped under the KEK, tag verified on
every open.

**Every write in this feature and its audit row share one transaction (FR-029).** If the
audit insert fails, the change rolls back. This is not a convention the handlers follow by
habit — the store's write methods take a transaction and perform both inserts inside it,
so there is no code path that can write one without the other.

Key rotation re-wraps `wrapped_dek` for every row and never touches `cipher_value`, which
is the property Principle VII requires of the scheme.

---

## Entity relationships

```text
account ──created_by/updated_by/archived_by──▶ property
property_type ──1:N──▶ property                (RESTRICT)
area          ──1:N──▶ property                (RESTRICT)

property ──1:N──▶ property_field_value        (RESTRICT)
property ──1:N──▶ property_field_multi_value  (RESTRICT)

custom_field ──1:N──▶ custom_field_choice        (RESTRICT)
custom_field ──1:N──▶ property_field_value       (RESTRICT)
custom_field ──1:N──▶ property_field_multi_value (RESTRICT)
custom_field_choice ──1:N──▶ property_field_value       (RESTRICT)
custom_field_choice ──1:N──▶ property_field_multi_value (RESTRICT)

audit_log ──entity_type + entity_id──▶ any of the above (no FK, by design:
    the log outlives nothing, but it must never constrain what it records)
```

`audit_log.entity_id` carries no foreign key deliberately. A referential constraint from
the audit table would let a business record's existence be inferred from a failed delete,
and more importantly would give the audit table a say in whether a change may proceed. The
log records; it does not vote.


---

## 0015 — Correcting the seeded areas

Migration 0011 seeded Cairo districts. The areas belong to an Assiut diocese, per
research.md D-011, which said they come from the design source. This was caught in review
after 0011 had already been applied, and Constitution IV prohibits editing an applied
migration — so the correction ships forward.

```sql
-- Renames, never deletes, the areas that are in use: six of the eight seeded rows are
-- each referenced by a seeded property, and area_id is ON DELETE RESTRICT, so a DELETE
-- would fail. UPDATE keeps every foreign key valid.
UPDATE area SET label = 'وسط البلد، أسيوط', label_normalized = 'وسط البلد، اسيوط' WHERE ...
-- ... five more, then:
-- Removes only areas nothing references.
DELETE FROM area a WHERE NOT EXISTS (SELECT 1 FROM property p WHERE p.area_id = a.id);
```

Two things this migration exposed, both worth keeping written down:

- **Migration 0011 stored `label_normalized` un-normalised.** It inserted the input string
  into both columns, so `مصر الجديدة` was stored with `label_normalized = 'مصر الجديدة'`
  rather than `'مصر الجديده'`. Every row 0011 seeded carried this. It mattered only for
  uniqueness — a later `مصر الجديده` would not have collided with it — and 0015 resolves
  it for the areas by writing correct normalised values. The two seeded property types
  (تجاري، سكني) contain no alef, ya, or ta-marbuta variants, so their normalised form is
  identical to their label and they are unaffected. Every row created through the API runs
  `identity.Canonical` first and is correct.
- **`label_normalized` values in 0015 are literals, not computed in SQL.** They were
  produced by running the Go normaliser and pasting its output. Research D-007 rejects a
  second normalisation rule in SQL precisely because the two would drift; a migration is no
  exception.

The `-- +goose Down` for this migration is best-effort. It flips the labels back, but if a
user has renamed one of these areas through the lookup API between Up and Down, the match
can miss. It never drops a table or invents data.
