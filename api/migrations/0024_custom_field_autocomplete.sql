-- +goose NO TRANSACTION
--
-- The 'autocomplete' custom-field type: free text stored exactly like 'text'
-- (property_field_value.text_value), whose only distinction is that the client
-- offers values already entered for the same field elsewhere in the register.
-- It can never be sensitive — constraint custom_field_sensitive_text_only_chk
-- from migration 0013 already restricts sensitivity to 'text', so no schema
-- change is needed for that rule.
--
-- This MUST carry NO TRANSACTION like 0019 before it: PostgreSQL refuses to
-- *use* a new enum value inside the transaction that added it, and goose wraps
-- each migration in one by default. Nothing here uses the value yet — the
-- application starts accepting it only after this migration has committed —
-- but the header keeps the pattern uniform so a later edit cannot trip over it.

-- +goose Up
-- +goose StatementBegin
ALTER TYPE custom_field_type ADD VALUE IF NOT EXISTS 'autocomplete';

-- The type-ahead endpoint asks for every stored value of ONE custom field,
-- then for the per-choice usage counts of a dropdown/multiselect field. Both
-- value tables have primary keys led by property_id, so a query constrained
-- only by custom_field_id would seq-scan them on every keystroke. These
-- indexes let those queries touch just the one field's rows. The register-wide
-- suggestions (code, name, lookups, attachment descriptions) aggregate the
-- whole table and read every row whatever an index offers, so they gain
-- nothing from one and get none.
CREATE INDEX IF NOT EXISTS property_field_value_field_idx
    ON property_field_value(custom_field_id);
CREATE INDEX IF NOT EXISTS property_field_multi_value_field_idx
    ON property_field_multi_value(custom_field_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- PostgreSQL cannot remove a value from an enum; rebuilding the type would
-- mean rewriting custom_field rows that reference it. As with 0019, the value
-- is left in place — inert once the application stops offering it. The two
-- indexes this migration added are dropped normally.
DROP INDEX IF EXISTS property_field_multi_value_field_idx;
DROP INDEX IF EXISTS property_field_value_field_idx;
SELECT 1;
-- +goose StatementEnd
