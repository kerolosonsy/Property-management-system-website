-- +goose Up
-- +goose StatementBegin

CREATE TABLE session (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id   uuid        NOT NULL REFERENCES account(id) ON DELETE RESTRICT,
    token_sha256 bytea       NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz NULL,

    CONSTRAINT session_token_uniq UNIQUE (token_sha256)
);

CREATE INDEX session_account_id_idx ON session(account_id);
CREATE INDEX session_last_seen_at_idx ON session(last_seen_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS session;
-- +goose StatementEnd
