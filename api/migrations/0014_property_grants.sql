-- +goose Up
-- +goose StatementBegin

-- Privileges for the new tables. audit_log is NOT mentioned and that is the
-- point: it keeps the SELECT, INSERT grant from feature 001; this feature
-- cannot update or delete a record even by mistake.

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE property                   TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE property_type              TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE area                       TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE custom_field               TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE custom_field_choice        TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE property_field_value       TO pms_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE property_field_multi_value TO pms_app;
GRANT USAGE ON SEQUENCE property_code_seq                                TO pms_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

REVOKE USAGE ON SEQUENCE property_code_seq                                FROM pms_app;
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE property_field_multi_value FROM pms_app;
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE property_field_value       FROM pms_app;
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE custom_field_choice        FROM pms_app;
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE custom_field               FROM pms_app;
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE area                       FROM pms_app;
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE property_type              FROM pms_app;
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE property                   FROM pms_app;

-- +goose StatementEnd
