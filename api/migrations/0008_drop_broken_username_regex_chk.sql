-- +goose Up
-- +goose StatementBegin

-- Defect fix: 0001_enums_and_account.sql set:
--
--     CONSTRAINT account_username_chars_chk
--         CHECK (username ~ '^[\p{L}\p{Nd}._-]+$')
--
-- PostgreSQL's POSIX-flavored "Advanced Regular Expressions" parser accepts
-- this at CREATE TABLE time but raises
--   "invalid regular expression: invalid escape \ sequence"
-- when the constraint is actually evaluated at INSERT time, because
-- PostgreSQL does not implement the Perl/PCRE Unicode property classes
-- (\p{L}, \p{Nd}). The result is that no INSERT into `account` succeeds.
--
-- The constitution forbids editing an applied migration (Principle IV), so
-- this migration drops the broken constraint. Character-class validation for
-- FR-036 is enforced in the Go layer (`validateUsername`,
-- `identity.AllowedUsernameRune`), which the database never sees as a
-- security boundary anyway. The `char_length` and uniqueness checks remain.

ALTER TABLE account DROP CONSTRAINT IF EXISTS account_username_chars_chk;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Recreate without the broken Unicode-property regex. A simpler check that
-- PostgreSQL CAN evaluate is to forbid whitespace and a couple of obvious
-- control characters. Application-layer validation is the real gate.
ALTER TABLE account
    ADD CONSTRAINT account_username_chars_chk
    CHECK (username !~ '[[:space:]\x00-\x1F\x7F]');

-- +goose StatementEnd