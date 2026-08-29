# Implementation Plan: Project Foundation & Login

**Branch**: `001-project-setup-login` | **Date**: 2026-08-29 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-project-setup-login/spec.md`

## Summary

Stand up the monorepo, the local runtime, and everything needed to sign in and manage
who may sign in. Four user stories: signing in (P1), administering accounts (P2),
ending sessions safely (P3), and reviewing recorded actions (P4).

Technical approach: a Go service exposing REST/JSON over TLS, described by an OpenAPI
document that generates the Angular client; an Angular single-page application in
Arabic with a right-to-left layout; PostgreSQL in a container holding accounts,
sessions, failure counters, and an append-only audit log. Sessions are opaque
random tokens stored hashed and delivered in an `HttpOnly` cookie; the session carries
identity only, and the account's role and active state are read on every request so a
demotion or deactivation takes effect immediately. Passwords are Argon2id. Repeated
failures are slowed by a doubling wait rather than a lockout. The audit table is
written in the same transaction as the change it records, and the application's
database role is granted `INSERT` and `SELECT` on it and nothing else.

## Technical Context

**Language/Version**: Go 1.27 (installed) for the API; TypeScript on Node 22.23 for the
web application; Angular pinned to the latest stable release at scaffold time and
recorded in `research.md` once scaffolded.

**Primary Dependencies**:

- API: Go standard library `net/http` for routing (method-and-pattern routing, no
  third-party router), `jackc/pgx/v5` for PostgreSQL, `golang.org/x/crypto/argon2` for
  password hashing, `golang.org/x/text` for Unicode normalization, `pressly/goose` for
  migrations, `oapi-codegen` to generate server types and the route interface from the
  OpenAPI document.
- Web: Angular with standalone components, Angular's `HttpClient`, and a client
  generated from the same OpenAPI document by `@openapitools/openapi-generator-cli`
  (`typescript-angular` generator; Java 23 is installed and available for it).

**Storage**: PostgreSQL 17 running in Docker (Docker 29.5.2 installed; no local
`psql` on this machine, so all database access is containerized).

**Testing**: Manual verification per Constitution Principle V. The procedure lives in
[quickstart.md](./quickstart.md). Automated tests are written only for the two pieces
whose correctness cannot be judged by looking at a screen: Arabic username
canonicalization and the progressive-delay schedule.

**Target Platform**: A single developer machine (macOS on arm64 here, Linux equally
supported), serving a handful of staff on one local network. No production target.

**Project Type**: Web application — Angular front end plus Go API in one repository,
with a shared OpenAPI contract.

**Performance Goals**: SC-001 requires sign-in to complete in under 15 seconds of
human time. Technically: non-authentication requests answer in under 200 ms at the
95th percentile; the sign-in request is deliberately slower, roughly 100–400 ms, being
dominated by Argon2id, plus any progressive wait already owed.

**Constraints**:

- TLS on every listener including local development (Principle VII); no plaintext HTTP
  listener exists at all.
- Every response and every stored value uses Western Arabic digits; Eastern Arabic
  digits are converted on entry (FR-032, FR-033).
- The session must never be the authority on role (FR-034).
- Audit rows must be unmodifiable by the application's database role (FR-024).
- No secret in version control (FR-031).

**Scale/Scope**: Tens of accounts, one installation, no horizontal scaling. Audit rows
grow at roughly the rate of staff activity — thousands per year, not millions. 48
functional requirements, 16 success criteria, 4 user stories, one OpenAPI document with
11 operations, 4 database tables.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Derived from `.specify/memory/constitution.md` v1.1.0.

**Initial evaluation (before Phase 0)** — all gates PASS:

- **I. Authorization** — PASS. Every operation in the contract carries an explicit role
  requirement; role is resolved per request from stored data, never from the session or
  from any client-supplied value. Angular guards are declared as presentation only.
- **II. Arabic RTL** — PASS. `lang="ar"`, `dir="rtl"`, logical CSS properties
  throughout. No imported design is consumed by this feature yet, so no RTL conversion
  task is owed here (see Risks).
- **III. Contract-First** — PASS. `contracts/openapi.yaml` is written in Phase 1, before
  any handler; both the Go server interface and the Angular client are generated from
  it.
- **IV. Data Integrity** — PASS. Real foreign keys, forward-only goose migrations,
  `timestamptz` in UTC. No money in this feature, so the monetary-type rule is not
  exercised.
- **V. Manual Verification** — PASS. `quickstart.md` carries the step-by-step
  procedure, including manager-denied-configuration steps.
- **VI. Simplicity** — PASS. Standard-library routing, no service or repository layer
  beyond one package per concern, no query-builder or ORM, no caching layer.
- **VII. Encryption** — PASS with a scope note. TLS everywhere, Argon2id passwords,
  `HttpOnly`/`Secure`/`SameSite` cookies, secrets from the environment. This feature
  handles no attachments and no national ID or bank fields, so envelope encryption and
  column encryption are specified in the constitution but not exercised here; the key
  material and its environment variable are still established now so the first feature
  that uploads a file inherits a working key path rather than inventing one.
- **VIII. Audit** — PASS. Every write listed in FR-022 produces a row in the same
  transaction; the application role holds `INSERT` and `SELECT` only.

**Post-design re-evaluation (after Phase 1)** — all gates still PASS. Recorded in
[research.md](./research.md) §11. No violation arose during design, so Complexity
Tracking below is empty.

## Project Structure

### Documentation (this feature)

```text
specs/001-project-setup-login/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output — decisions and rejected alternatives
├── data-model.md        # Phase 1 output — tables, constraints, state transitions
├── quickstart.md        # Phase 1 output — setup and manual verification procedure
├── contracts/
│   └── openapi.yaml     # Phase 1 output — the API contract, source of truth
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output — created by /speckit-tasks, not by this command
```

### Source Code (repository root)

```text
api/                              # Go service
├── cmd/
│   ├── server/main.go            # Loads config, opens the pool, serves TLS
│   └── admintool/main.go         # Offline utility: seed and recover the administrator
├── internal/
│   ├── config/                   # Environment variable loading and validation
│   ├── httpx/                    # Router wiring, middleware, error body shape
│   ├── auth/                     # Argon2id, session issue and verify, progressive delay
│   ├── identity/                 # Accounts, roles, username canonicalization
│   ├── audit/                    # Audit writer and the records query
│   └── gen/                      # Generated from contracts/openapi.yaml — never hand-edited
├── migrations/                   # Forward-only goose SQL migrations
└── go.mod

web/                              # Angular application
├── src/
│   ├── app/
│   │   ├── core/                 # Session state, HTTP interceptors, route guards
│   │   ├── shared/               # Digit-normalizing directive, Arabic error messages
│   │   ├── features/
│   │   │   ├── sign-in/          # User Story 1
│   │   │   ├── users/            # User Story 2
│   │   │   └── records/          # User Story 4
│   │   └── api/                  # Generated client — never hand-edited
│   ├── styles/                   # Logical-property base styles, RTL tokens
│   └── index.html                # lang="ar" dir="rtl"
└── package.json

contracts/
└── openapi.yaml                  # Copied from the feature's contracts/ on acceptance;
                                  # the repository-level source of truth thereafter

infra/
├── docker-compose.yml            # PostgreSQL 17 only — the app runs on the host
└── certs/                        # Locally generated TLS material — gitignored

.env.example                      # Variable names, no values
```

**Structure Decision**: Constitution's monorepo layout — `web/`, `api/`, `contracts/`,
`specs/` — with `infra/` added for the database container and local certificates. The
Go service uses one package per concern under `internal/` rather than a layered
service/repository split, because at this size a repository interface with exactly one
implementation is indirection without benefit (Principle VI). Generated code lives in
its own directory in both projects so that "never hand-edit" is visible from the path.

## Complexity Tracking

No Constitution Check violations. This section is intentionally empty.

## Risks & Dependencies

| Risk | Effect | Handling |
|------|--------|----------|
| ~~The Claude Design source has not been imported~~ — **resolved 2026-08-29** | — | Imported and applied (T082). The design system's tokens are vendored to `web/src/styles/design-system.css` and the application's `.pms-*` classes map onto them. The source proved **already Arabic and already RTL** (`dir="rtl" lang="ar"`), so no language or direction conversion was needed; its stylesheet did use four physical CSS properties, which the vendored copy converts. |
| The Angular major version is not yet pinned | Scaffolding could pull a version whose APIs differ from what tasks assume | Pin at scaffold time, record the exact version in `research.md`, and commit the lockfile in the same task. |
| No PostgreSQL client on the host | Migration and inspection steps could silently assume `psql` | Every database command in `quickstart.md` runs through the container. |
| Self-signed certificate causes a browser warning on first use | A developer may assume the app is broken | `quickstart.md` states the expected warning and how to proceed, as an expected outcome rather than a failure. |
| The administrator recovery utility is a credential-bypassing tool | Misuse or accidental exposure | It is a separate binary requiring direct database credentials and shell access on the machine; it is never reachable over HTTP, and each use writes an audit row. |
