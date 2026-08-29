-- +goose Up
-- +goose StatementBegin

-- The login_failure table drives the progressive delay (FR-014..FR-016, FR-040,
-- FR-042). It is deliberately keyed by the submitted canonical username — NOT
-- linked to the account table — so unknown usernames accumulate delay identically
-- to real ones.
CREATE TABLE login_failure (
    username_canonical   text        PRIMARY KEY,
    consecutive_failures integer     NOT NULL DEFAULT 0,
    last_failure_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX login_failure_last_idx ON login_failure(last_failure_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS login_failure;
-- +goose StatementEnd
