-- +goose Up
-- +goose StatementBegin

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'audit_action') THEN
        CREATE TYPE audit_action AS ENUM (
            'sign_in_succeeded',
            'sign_in_failed',
            'sign_out',
            'password_changed',
            'password_reset',
            'account_created',
            'account_role_changed',
            'account_activated',
            'account_deactivated',
            'sessions_invalidated',
            'admin_recovery_used'
        );
    END IF;
END$$;

CREATE TABLE audit_log (
    id                     bigserial    PRIMARY KEY,
    occurred_at            timestamptz  NOT NULL DEFAULT now(),
    action                 audit_action NOT NULL,
    actor_account_id       uuid         NULL REFERENCES account(id) ON DELETE RESTRICT,
    actor_username_snapshot text        NOT NULL,
    actor_role             account_role NULL,
    target_account_id      uuid         NULL REFERENCES account(id) ON DELETE RESTRICT,
    target_username_snapshot text       NULL,
    source_ip              inet         NOT NULL,
    detail                 jsonb        NULL
);

CREATE INDEX audit_log_occurred_at_id_idx ON audit_log(occurred_at DESC, id DESC);
CREATE INDEX audit_log_actor_id_idx       ON audit_log(actor_account_id);
CREATE INDEX audit_log_action_idx         ON audit_log(action);
CREATE INDEX audit_log_target_id_idx      ON audit_log(target_account_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS audit_log;
DROP TYPE  IF EXISTS audit_action;
-- +goose StatementEnd
