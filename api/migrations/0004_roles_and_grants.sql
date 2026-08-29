-- +goose Up
-- +goose StatementBegin

-- Application database role. The running Go service connects as this role. The owner
-- role remains separate (the one that ran this migration), and its credentials never
-- appear in the running service's environment (Constitution VIII).

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'pms_app') THEN
        CREATE ROLE pms_app LOGIN PASSWORD 'dev_only_app_pw';
    END IF;
END$$;

GRANT CONNECT ON DATABASE pms TO pms_app;
GRANT USAGE   ON SCHEMA public TO pms_app;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE account  TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE session  TO pms_app;

-- Constitution VIII / FR-024: application role may only SELECT and INSERT on
-- audit_log. UPDATE and DELETE are never granted.
GRANT SELECT, INSERT ON TABLE audit_log TO pms_app;

GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO pms_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pms_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO pms_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
REVOKE ALL PRIVILEGES ON ALL TABLES    IN SCHEMA public FROM pms_app;
REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM pms_app;
-- (Keep the role; its existence is harmless, and dropping it from a migration
--  makes the down-migration brittle when other databases share the role name.)
-- +goose StatementEnd
