# Property Management System — Go API

The API for the Property Management System. Generated, never hand-edited
contracts and migrations sit alongside hand-written code in this directory.

## Layout

```text
api/
├── cmd/
│   ├── server/        # The HTTPS-only service (FR-029).
│   ├── admintool/     # Offline utility — seed the first administrator and
│   │                  # recover the administrator password. Uses the OWNER
│   │                  # role; never reachable over HTTP.
│   └── migrate/       # Goose migration runner; OWNER role only.
├── internal/
│   ├── auth/          # Argon2id, opaque session tokens, progressive delay.
│   ├── audit/         # Append-only audit writer and records query.
│   ├── config/        # Environment loading and validation.
│   ├── db/            # pgx pool — opens as the application role.
│   ├── gen/           # Generated from contracts/openapi.yaml. Never edited.
│   ├── httpx/         # Router, middleware, error envelope, handlers.
│   └── identity/      # Account storage and Arabic username canonicalization.
├── migrations/        # Forward-only goose SQL migrations. Never edited once
│                      # applied (Constitution IV).
└── oapi-codegen.yaml  # Code-gen config for `make generate`.
```

## Commands

```bash
# Generate server types and the route interface from the contract.
make generate

# Apply migrations using the owner role (PMS_DATABASE_OWNER_URL must be set).
make migrate

# Start the API over TLS only — refuses to start if cert or key is missing.
make run-api

# Run the two pieces that carry unit tests.
go test ./internal/identity/... ./internal/auth/...
```

## Roles and privileges

Two database roles exist:

- **`pms_owner`** runs migrations and the `admintool`. Its credentials are
  only in the migrate/admintool environment, never in the running service.
- **`pms_app`** is what the running service connects as. It holds
  `SELECT, INSERT, UPDATE, DELETE` on the business tables (`account`,
  `session`, `login_failure`) and **only `SELECT, INSERT` on `audit_log`**
  (Constitution VIII). `UPDATE` and `DELETE` are never granted on
  `audit_log`. Default privileges are narrowed to `SELECT, INSERT` so future
  tables do not silently inherit write rights (`migrations/0007_…`).

The application never opens a plaintext HTTP socket. The single listener is
`ListenAndServeTLS`; `config.Load` fails if either the cert or the key is
missing, so a misconfigured environment cannot fall back to HTTP.

## Generated code

`api/internal/gen/` is created by `oapi-codegen` from
`contracts/openapi.yaml`. The build fails if the generated types or the
`ServerInterface` drift from the contract — that is what makes Constitution
III enforceable rather than aspirational. Edit the contract, then regenerate.