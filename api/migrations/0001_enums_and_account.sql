-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'account_role') THEN
        CREATE TYPE account_role AS ENUM ('admin', 'manager');
    END IF;
END$$;

CREATE TABLE account (
    id                   uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    username             text          NOT NULL,
    username_canonical   text          NOT NULL,
    display_name         text          NOT NULL,
    password_hash        text          NOT NULL,
    role                 account_role  NOT NULL,
    is_active            boolean       NOT NULL DEFAULT true,
    must_change_password boolean       NOT NULL DEFAULT true,
    created_at           timestamptz   NOT NULL DEFAULT now(),
    updated_at           timestamptz   NOT NULL DEFAULT now(),
    created_by           uuid          NULL REFERENCES account(id) ON DELETE RESTRICT,

    CONSTRAINT account_username_length_chk
        CHECK (char_length(username) BETWEEN 3 AND 32),
    CONSTRAINT account_username_chars_chk
        CHECK (username ~ '^[\p{L}\p{Nd}._-]+$'),
    CONSTRAINT account_display_name_length_chk
        CHECK (char_length(display_name) BETWEEN 1 AND 120),
    CONSTRAINT account_username_canonical_uniq
        UNIQUE (username_canonical)
);

CREATE INDEX account_role_active_idx ON account(role) WHERE is_active;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS account;
DROP TYPE  IF EXISTS account_role;
-- +goose StatementEnd
