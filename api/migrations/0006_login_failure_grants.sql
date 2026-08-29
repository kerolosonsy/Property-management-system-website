-- +goose Up
-- +goose StatementBegin

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE login_failure TO pms_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
REVOKE ALL PRIVILEGES ON TABLE login_failure FROM pms_app;
-- +goose StatementEnd
