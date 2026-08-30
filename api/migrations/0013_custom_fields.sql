-- +goose Up
-- +goose StatementBegin

-- Custom field definitions, choices, and values. ON DELETE RESTRICT everywhere
-- is FR-027g: a field, a type change, or a choice removal is refused while any
-- property holds a value that the change would destroy.
--
-- The two value-shape constraints make a row hold exactly one kind of value,
-- and the four cipher columns all-or-nothing.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'custom_field_type') THEN
        CREATE TYPE custom_field_type AS ENUM ('text', 'dropdown', 'multiselect', 'checkbox');
    END IF;
END$$;

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

CREATE TABLE property_field_value (
    property_id     uuid    NOT NULL REFERENCES property(id)     ON DELETE RESTRICT,
    custom_field_id uuid    NOT NULL REFERENCES custom_field(id) ON DELETE RESTRICT,

    text_value      text    NULL,
    bool_value      boolean NULL,
    choice_id       uuid    NULL REFERENCES custom_field_choice(id) ON DELETE RESTRICT,

    cipher_value    bytea   NULL,
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

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS property_field_multi_value;
DROP TABLE IF EXISTS property_field_value;
DROP TABLE IF EXISTS custom_field_choice;
DROP TABLE IF EXISTS custom_field;
DROP TYPE  IF EXISTS custom_field_type;

-- +goose StatementEnd
