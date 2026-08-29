# Property Management System — Angular Web

The Arabic, right-to-left Angular single-page application. Standalone
components, no SSR, the HTTP client generated from `contracts/openapi.yaml`.

## Layout

```text
web/src/
├── app/
│   ├── api/            # Generated client — never hand-edited.
│   ├── core/           # Session state, HTTP interceptor, route guards.
│   ├── shared/         # Eastern-to-Western digit directive, Arabic messages.
│   ├── features/
│   │   ├── sign-in/           # User Story 1 — sign-in + forced password change.
│   │   ├── users/             # User Story 2 — list / create / edit.
│   │   └── records/           # User Story 4 — recorded-actions viewer.
│   ├── app.config.ts    # Provides router, HttpClient, and the API config.
│   ├── app.routes.ts    # Three feature routes + sign-in + change-password.
│   └── app.ts           # Shell with toolbar (sign-out, role-gated nav).
├── styles.scss          # Logical-CSS base. RTL follows dir="rtl" structurally.
└── index.html           # lang="ar" dir="rtl".
```

## Commands

```bash
# Regenerate the client from the contract.
npm --prefix web run gen:api

# Install dependencies.
npm --prefix web ci

# Start the dev server (proxies /api/v1 to https://localhost:8443).
npm --prefix web start

# Build for production.
npm --prefix web run build
```

## RTL, Arabic, and digits

- `index.html` declares `lang="ar" dir="rtl"`.
- Every stylesheet uses logical properties (`margin-inline-start`,
  `padding-inline-end`, `inset-inline-*`, `text-align: start`). Mirroring is
  structural; no per-component `left` / `right` patches.
- The `appWesternDigits` directive converts Eastern Arabic digits to Western
  digits as the user types (FR-033). The server also converts on receipt, so
  the API is correct even when called without the browser.

## Authorization

Route guards and hidden menu items are presentation only (Constitution I).
The server enforces every role check, and a manager cookie that reaches
`/users` or `/audit-records` over the network is refused with 403 regardless
of what the UI shows.

## Generated code

`web/src/app/api/` is created by `@openapitools/openapi-generator-cli` from
`contracts/openapi.yaml`. Edit the contract, then run `npm run gen:api`.