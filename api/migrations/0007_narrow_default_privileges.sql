-- +goose Up
-- +goose StatementBegin

-- Defect fix: 0004_roles_and_grants.sql set:
--
--     ALTER DEFAULT PRIVILEGES IN SCHEMA public
--         GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pms_app;
--
-- Default privileges apply only to FUTURE tables created by the owner role.
-- Any audit-style or business table added by a later migration would silently
-- inherit UPDATE and DELETE on the application role, defeating Constitution
-- VIII (the application's database role must NOT be able to mutate audit
-- rows). This migration narrows the default to SELECT, INSERT only.
--
-- The previously-granted UPDATE/DELETE on existing tables is preserved for
-- business tables (account, session, login_failure) — those are explicitly
-- named in earlier migrations. Tables added later must opt in explicitly.

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    REVOKE UPDATE, DELETE ON TABLES FROM pms_app;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT ON TABLES TO pms_app;

-- Sequence defaults: keep USAGE, SELECT (needed for SERIAL/BIGSERIAL keys).
-- Already set in 0004; reasserted here for clarity.

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Reverting brings back the broad default. We do NOT do that automatically
-- because the down-migration is a rollback path; if 0007 has been applied
-- for some time, newer migrations may rely on the narrowed default. The
-- down-migration is intentionally a no-op for safety; rerun 0004 manually
-- if you must restore the broad default.
DO $$ BEGIN
    -- intentionally empty
END $$;

-- +goose StatementEnd