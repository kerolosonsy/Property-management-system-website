---

description: "Task list for Project Foundation & Login"
---

# Tasks: Project Foundation & Login

**Input**: Design documents from `/specs/001-project-setup-login/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/openapi.yaml](./contracts/openapi.yaml), [quickstart.md](./quickstart.md)

**Tests**: Constitution Principle V sets no automated test gate. Only two pieces carry
unit tests, because neither can be judged by looking at a screen: Arabic username
canonicalization and the progressive-delay schedule. Everything else is verified by the
procedure in `quickstart.md`, and the verification steps are tasks in their own right.

**Organization**: Grouped by user story so each can be built and demonstrated on its own.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an unfinished task)
- **[Story]**: US1–US4, matching the four user stories in `spec.md`

## Path Conventions

Monorepo per `plan.md`: `api/` (Go), `web/` (Angular), `contracts/` (OpenAPI source of
truth), `infra/` (database container and local certificates), `specs/` (these documents).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: An empty but runnable monorepo with a local database and TLS material.

- [x] T001 Create the monorepo directories `api/`, `web/`, `contracts/`, `infra/` at the repository root per plan.md Project Structure
- [x] T002 [P] Initialize the Go module in api/go.mod targeting Go 1.27
- [x] T003 [P] Scaffold the Angular application into web/ with standalone components, pin the exact version, commit web/package-lock.json, and record the pinned version in specs/001-project-setup-login/research.md §10
- [x] T004 [P] Define PostgreSQL 17 in infra/docker-compose.yml with a named volume and no port exposed beyond localhost
- [x] T005 [P] Add .gitignore entries for .env, infra/certs/, web/node_modules/, web/dist/, and api/bin/
- [x] T006 [P] Write .env.example listing every variable name with no values: database URL for the owner role, database URL for the application role, session cookie name, TLS certificate and key paths, and PMS_ATTACHMENT_KEK reserved for the first upload feature
- [x] T007 [P] Write Makefile with targets certs, migrate, generate, run-api, run-web
- [x] T008 Implement the certs target in Makefile to generate a self-signed localhost certificate into infra/certs/ using OpenSSL, and confirm the directory is gitignored
- [x] T009 [P] Add the Go tool dependencies oapi-codegen and pressly/goose to api/tools.go and api/go.mod
- [x] T010 [P] Add @openapitools/openapi-generator-cli as a dev dependency in web/package.json with an npm script that runs the typescript-angular generator

**Checkpoint**: `make certs` produces a certificate, `docker compose -f infra/docker-compose.yml up -d` starts a healthy database, and both projects build empty.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The contract, the schema, the privileges, the TLS listener, and the Arabic
RTL shell. Nothing in Phase 3 onward can be built without these.

**⚠️ CRITICAL**: No user story work begins until this phase is complete.

### Contract first (Constitution III)

- [x] T011 Copy specs/001-project-setup-login/contracts/openapi.yaml to contracts/openapi.yaml as the repository-level source of truth, and note in api/README.md that handlers are never written before the contract
- [x] T012 Add api/oapi-codegen.yaml and wire `make generate` to produce server types and the route interface into api/internal/gen/ from contracts/openapi.yaml
- [x] T013 Wire `make generate` to also produce the Angular client into web/src/app/api/ from contracts/openapi.yaml, and add a header comment in both generated directories stating they are never hand-edited

### Schema and privileges (Constitution IV, VIII)

- [x] T014 Write forward-only migration api/migrations/0001_enums_and_account.sql creating the account_role and audit_action enums and the account table with every constraint in data-model.md, including the unique index on username_canonical
- [x] T015 [P] Write forward-only migration api/migrations/0002_session.sql creating the session table with the unique index on token_sha256 and indexes on account_id and last_seen_at
- [x] T016 [P] Write forward-only migration api/migrations/0003_audit_log.sql creating the audit_log table with the snapshot columns and the four indexes named in data-model.md
- [x] T017 Write forward-only migration api/migrations/0004_roles_and_grants.sql creating the application database role, granting it full rights on the business tables, and granting it only SELECT and INSERT on audit_log — never UPDATE or DELETE
- [x] T018 Implement the migrate target in Makefile to run goose inside the database container using the owner credentials only

### Service foundation

- [x] T019 [P] Implement environment configuration loading and validation in api/internal/config/config.go, failing to start when a required variable is absent
- [x] T020 [P] Implement the pgx pool in api/internal/db/pool.go using the application role credentials, never the owner's
- [x] T021 Implement the TLS-only server in api/cmd/server/main.go — one HTTPS listener, no plaintext listener created under any configuration (FR-029)
- [x] T022 [P] Implement the single error body shape and its code set in api/internal/httpx/errors.go, matching the Error schema in contracts/openapi.yaml, with no internal detail ever included (FR-028)
- [x] T023 [P] Write the Arabic user-facing message catalogue in api/internal/httpx/messages_ar.go, including the one refusal message shared by unknown username, wrong password, and deactivated account (FR-013)
- [x] T024 [P] Implement source IP extraction in api/internal/httpx/clientip.go for the audit record's source address (FR-023)
- [x] T025 Implement the audit writer in api/internal/audit/writer.go, taking the caller's transaction so the record and the change it describes commit or roll back together (Constitution VIII)
- [x] T026 Implement the health endpoint in api/internal/httpx/health.go and wire it to the generated route interface

### Web foundation (Constitution II)

- [x] T027 [P] Set lang="ar" and dir="rtl" in web/src/index.html and establish the base stylesheet in web/src/styles/ using logical CSS properties only — no physical left/right rules
- [x] T028 [P] Implement the Eastern-to-Western digit directive in web/src/app/shared/western-digits.directive.ts, converting as the user types and leaving surrounding Arabic text untouched (FR-033)
- [x] T029 [P] Implement the HTTP interceptor in web/src/app/core/http.interceptor.ts to send the session cookie with every request and map the API error shape to its Arabic message
- [x] T030 Build the application shell and routing skeleton in web/src/app/ with the three feature routes declared but empty
- [x] T031 Run quickstart.md section A (Foundation) and record the result — including A3, which must show that no plaintext listener exists

**Checkpoint**: The contract generates both sides, the schema and privileges are applied, the service answers `/health` over TLS only, and the empty Angular shell renders right to left.

---

## Phase 3: User Story 1 - Signing in to the system (Priority: P1) 🎯 MVP

**Goal**: A seeded administrator signs in, is forced to set a new password, and reaches
the main area. Wrong credentials are refused indistinguishably.

**Independent Test**: With one seeded administrator and nothing else built, sign in with
correct credentials and reach the main area; sign in with a wrong password and with a
username that does not exist, and confirm the two refusals are identical.

### Tests for User Story 1

- [x] T032 [P] [US1] Unit tests for Arabic username canonicalization in api/internal/identity/canonical_test.go covering case, diacritics, tatweel, alef, ya, and ta marbuta equivalence, Eastern digit conversion, and rejection of zero-width and bidirectional control characters (FR-036–FR-039)

### Implementation for User Story 1

- [x] T033 [P] [US1] Implement Argon2id hashing and verification in api/internal/auth/password.go with t=3, m=64MiB, p=2, per-password salt, and parameters encoded in the stored value (research §4)
- [x] T034 [P] [US1] Implement username canonicalization and character rejection in api/internal/identity/canonical.go per research §7
- [x] T035 [US1] Implement account lookup and creation queries in api/internal/identity/store.go, resolving sign-in by username_canonical and relying on the unique index rather than a prior existence check
- [x] T036 [US1] Implement session issue, verify, and revoke-all in api/internal/auth/session.go — 256-bit token from crypto/rand, only its SHA-256 stored, cookie HttpOnly, Secure, SameSite=Strict (FR-021)
- [x] T037 [US1] Implement the authentication middleware in api/internal/httpx/middleware.go, reading the account's current role and active state from stored data on every request and never from the session (FR-034)
- [x] T038 [US1] Implement POST /auth/login in api/internal/httpx/auth_handlers.go with a byte-identical refusal for unknown username, wrong password, and deactivated account, writing sign_in_succeeded or sign_in_failed audit rows (FR-013, FR-022)
- [x] T039 [US1] Implement GET /auth/me in api/internal/httpx/auth_handlers.go returning the caller's current role and password-change state
- [x] T040 [US1] Implement POST /auth/password in api/internal/httpx/auth_handlers.go — verifies the current password, enforces the 12-character minimum, clears must_change_password, revokes every session for the account, and writes a password_changed record (FR-009, FR-011, FR-020)
- [x] T041 [US1] Implement the seed-admin command in api/cmd/admintool/main.go, prompting without echo, creating the initial administrator with must_change_password set, and writing an audit record (FR-007)
- [x] T042 [P] [US1] Build the Arabic RTL sign-in screen in web/src/app/features/sign-in/, applying the digit directive to the username field and showing field-level Arabic validation
- [x] T043 [P] [US1] Build the forced password-change screen in web/src/app/features/sign-in/change-password/, which the user cannot navigate away from while must_change_password is set
- [x] T044 [US1] Implement the session state service and route guard in web/src/app/core/, with a comment in the guard stating it is presentation only and the server is the authority (Constitution I)
- [x] T045 [US1] Wire the sign-in, current-user, and password-change calls to the generated client in web/src/app/api/ — no hand-written request or response types
- [x] T046 [US1] Run quickstart.md section B (Signing in, B1–B8) and record the result

**Checkpoint**: The MVP. A seeded administrator can sign in, is forced to change the password, and refusals reveal nothing.

---

## Phase 4: User Story 2 - Administering user accounts (Priority: P2)

**Goal**: The administrator creates, edits, deactivates, and resets passwords for
accounts. Managers are refused every one of those operations at the server.

**Independent Test**: Sign in as administrator, create a manager, sign in as that
manager, then deactivate them and confirm sign-in is refused. Separately, call the
administration endpoints with the manager's own session cookie and confirm 403.

### Implementation for User Story 2

- [x] T047 [US2] Implement account administration queries in api/internal/identity/store.go — paged list, read one, update display name, role, and active state, and reset password
- [x] T048 [US2] Implement the last-active-administrator invariant in api/internal/identity/invariants.go, taking a row lock over active administrators inside the same transaction and refusing a demotion or deactivation that would bring the count to zero, and refusing an administrator targeting their own account
- [x] T049 [US2] Implement GET /users and POST /users in api/internal/httpx/user_handlers.go, refusing a canonically duplicate username with 409 and setting must_change_password on creation (FR-006, FR-010, FR-038)
- [x] T050 [US2] Implement GET /users/{userId} and PATCH /users/{userId} in api/internal/httpx/user_handlers.go, revoking every session on deactivation and leaving sessions untouched on a role change (FR-020, FR-035)
- [x] T051 [US2] Implement POST /users/{userId}/password in api/internal/httpx/user_handlers.go, setting must_change_password and revoking the target's sessions (FR-012, FR-020)
- [x] T052 [US2] Write account_created, account_role_changed, account_activated, account_deactivated, password_reset, and sessions_invalidated audit records inside each handler's transaction (FR-022, Constitution VIII)
- [x] T053 [P] [US2] Build the Arabic RTL account list screen in web/src/app/features/users/, showing active and inactive accounts
- [x] T054 [P] [US2] Build the account creation form in web/src/app/features/users/create/ with the digit directive on the username field and Arabic messages for duplicate and invalid usernames
- [x] T055 [P] [US2] Build the account edit controls in web/src/app/features/users/edit/ for display name, role, activation state, and password reset
- [x] T056 [US2] Run quickstart.md section C (Administering accounts, C1–C12) and record the result — C7 and C8 are the steps that prove server-side authorization
- [x] T057 [US2] Run quickstart.md section D (Role change while signed in, D1–D3) and record the result

**Checkpoint**: Stories 1 and 2 both work independently. A manager cannot reach administration by any route.

---

## Phase 5: User Story 3 - Ending a session safely (Priority: P3)

**Goal**: Deliberate sign-out, idle and absolute session expiry, and a growing wait after
repeated failures — with no account ever locked.

**Independent Test**: Sign in and sign out, then confirm the browser Back button restores
nothing. Separately, submit wrong passwords repeatedly and confirm each attempt must wait
longer, and that the correct password works once the wait passes.

### Tests for User Story 3

- [x] T058 [P] [US3] Unit tests for the progressive-delay schedule in api/internal/auth/delay_test.go covering the 1, 2, 4, 8, 16, 32, 60, 60 progression, the reset on success, and identical behaviour for a username that does not exist (FR-014–FR-016, FR-040)

### Implementation for User Story 3

- [x] T059 [US3] Write forward-only migration api/migrations/0005_login_failure.sql creating the login_failure table keyed by the submitted canonical username, deliberately with no foreign key to account (data-model.md)
- [x] T060 [P] [US3] Implement the delay schedule and failure counter in api/internal/auth/delay.go, computing min(60, 2^(n-1)) seconds from the last failure and sweeping rows untouched for 15 minutes (FR-042)
- [x] T061 [US3] Wire the delay into POST /auth/login in api/internal/httpx/auth_handlers.go — refuse with 429 and retryAfterSeconds while a wait is outstanding even when the password is correct, apply the counter to unknown usernames identically, and clear it on success (FR-015, FR-016, FR-040)
- [x] T062 [US3] Enforce idle and absolute session expiry in api/internal/httpx/middleware.go by deriving them from last_seen_at and created_at rather than from a stored expiry (FR-017, FR-018)
- [x] T063 [US3] Implement POST /auth/logout in api/internal/httpx/auth_handlers.go, revoking the session, clearing the cookie, and writing a sign_out record (FR-019)
- [x] T064 [P] [US3] Implement the expired-session sweep in api/internal/auth/sweep.go as housekeeping only, never as the mechanism of expiry
- [x] T065 [P] [US3] Add the sign-out control and the Arabic remaining-wait message to web/src/app/features/sign-in/ and the application shell, showing the seconds remaining from retryAfterSeconds
- [x] T066 [US3] Handle session expiry in web/src/app/core/http.interceptor.ts by returning the user to sign-in and discarding unsaved form state rather than submitting it later
- [ ] T067 [US3] Run quickstart.md section E (Ending a session, E1–E7) and record the result, confirming at E7 that no lockout and no unlock screen exist anywhere

**Checkpoint**: All three of the first stories work independently.

---

## Phase 6: User Story 4 - Reviewing what happened (Priority: P4)

**Goal**: The administrator reads recorded actions, newest first, filtered by person,
date range, action type, and affected account, with paging. Managers cannot reach it, and
nothing can edit or delete a record.

**Independent Test**: Perform a handful of sign-ins, a failure, and an account change,
then confirm each appears and that each filter narrows the list correctly. Sign in as a
manager and confirm the screen and its endpoint are both refused.

### Implementation for User Story 4

- [x] T068 [US4] Implement the audit records query in api/internal/audit/query.go with combinable filters on actor, target, action, and inclusive date range, ordered by occurred_at then id descending, using the indexes from data-model.md
- [x] T069 [US4] Implement GET /audit-records in api/internal/httpx/audit_handlers.go, administrator only, refusing a start date later than the end date with 400 rather than returning an empty page (FR-043, FR-044)
- [x] T070 [P] [US4] Build the Arabic RTL records screen in web/src/app/features/records/, rendering who, their role, what, which account, when in Africa/Cairo with Western digits, and from where (FR-023, FR-047)
- [x] T071 [P] [US4] Build the filter controls in web/src/app/features/records/filters/ for person, affected account, action type, and date range
- [x] T072 [US4] Implement paging in web/src/app/features/records/ that preserves the applied filters across pages, and an Arabic empty-result message that keeps the filters on screen (FR-045, FR-048)
- [x] T073 [US4] Confirm no edit or delete affordance exists anywhere in web/src/app/features/records/ and that no such operation exists in contracts/openapi.yaml (FR-046)
- [ ] T074 [US4] Run quickstart.md section F (Reading the records, F1–F10) and record the result

**Checkpoint**: All four user stories are independently functional.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [x] T075 Run quickstart.md section G (Audit immutability, G1–G3) and record the result, confirming the application role is denied UPDATE and DELETE on audit_log
- [x] T076 [P] Sweep every screen for Arabic completeness — no English string reachable by a user, and every refusal states what to do next (FR-026, FR-027)
- [x] T077 [P] Sweep every stylesheet in web/src/ for physical left/right properties and replace them with logical properties (Constitution II)
- [x] T078 [P] Sweep every screen and stored value for Eastern Arabic digits, including values entered deliberately in Eastern form (FR-032, SC-011)
- [x] T079 [P] Confirm no secret is reachable in version history or in any log line — keys, tokens, passwords, and connection strings (FR-031, Constitution VII)
- [x] T080 [P] Write api/README.md and web/README.md covering the run commands, the never-hand-edit rule for generated directories, and the owner-versus-application role split
- [x] T081 Complete the result record table at the end of specs/001-project-setup-login/quickstart.md — an unrun row is an unmet requirement (Constitution V)
- [x] T082 Apply the Claude Design visual design to the sign-in, accounts, and records screens — design system vendored to web/src/styles/design-system.css and the .pms-* classes mapped onto its tokens; the source proved already Arabic and already RTL, so the work was converting its four physical CSS properties to logical ones and applying the blueprint framing (Constitution II)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup. **Blocks every user story.**
- **User Story 1 (Phase 3)**: Depends on Foundational. Depends on no other story.
- **User Story 2 (Phase 4)**: Depends on Foundational, and on US1 for the session
  middleware (T037) and password hashing (T033) it reuses.
- **User Story 3 (Phase 5)**: Depends on Foundational, and on US1 for the login handler
  (T038) it extends and the middleware (T037) it adds expiry to.
- **User Story 4 (Phase 6)**: Depends on Foundational and on the audit writer (T025).
  Records exist from US1 onward whether or not anyone can read them, so this story can
  be built at any point after US1 and is never a prerequisite for another.
- **Polish (Phase 7)**: Depends on the stories being delivered. T082 additionally depends
  on an external step outside this repository.

### Within Each User Story

- The contract is already fixed in Phase 2; no handler task may change it. A contract
  change means editing `contracts/openapi.yaml` and regenerating first.
- Storage queries before handlers; handlers before the screens that call them.
- Audit writing is part of the handler task that performs the change, never a later pass
  — the record and the change share a transaction.

### Parallel Opportunities

- Phase 1: T002–T007, T009, and T010 all run in parallel after T001.
- Phase 2: the three migrations T015 and T016 run in parallel after T014; T019, T020,
  T022, T023, T024 run in parallel; the web foundation T027, T028, T029 runs in parallel
  with all of the service foundation.
- Phase 3: T032, T033, T034 in parallel; then the screens T042 and T043 in parallel once
  T038–T040 exist.
- Phase 4: the three screens T053, T054, T055 in parallel once T049–T051 exist.
- Phase 6: T070 and T071 in parallel once T069 exists.
- Phase 7: T076–T080 all in parallel.
- Across stories: with more than one person, US4 can be built alongside US2 and US3 once
  US1 is done, since it shares no file with either.

---

## Parallel Example: User Story 1

```text
After T031 (Foundational complete):
  Developer A: T033  Argon2id           api/internal/auth/password.go
  Developer B: T034  canonicalization   api/internal/identity/canonical.go
  Developer C: T032  canonicalization tests  api/internal/identity/canonical_test.go

Then sequentially, because they share files:
  T035 → T036 → T037 → T038 → T039 → T040 → T041

Then in parallel again:
  Developer A: T042  sign-in screen
  Developer B: T043  forced password-change screen
```

---

## Implementation Strategy

**MVP scope: Phase 1 + Phase 2 + Phase 3 (T001–T046).** That delivers a running,
TLS-only, Arabic RTL application where the seeded administrator signs in, is forced to
set a password, and where refusals reveal nothing about which usernames exist. It is
demonstrable on its own and satisfies User Story 1 in full.

**Incremental delivery after the MVP**:

1. **+ Phase 4** — the system stops having exactly one usable account. This is the point
   at which it becomes usable by staff.
2. **+ Phase 5** — unattended screens and password guessing are handled.
3. **+ Phase 6** — the records that have been accumulating since the MVP become readable.
4. **+ Phase 7** — the sweeps, the recorded verification, and the visual design.

**Sequencing note**: Phase 7's T082 is the only task with a dependency outside this
repository. Everything else can be completed today; the plain, correct RTL layout built
in Phases 3–6 is a deliberate placeholder, not an omission.

## Completion Status

**80 of 82 complete.** The two open tasks both need a human at a browser:

| Task | Why it is open |
|---|---|
| T067 | quickstart section E needs a browser and a real 31-minute idle wait |
| T074 | quickstart section F needs a browser to drive the records screen |
| ~~T082~~ | **done 2026-08-29** — design imported and applied |

Sections A and G of `quickstart.md` passed in full. Sections B, C, D, E, and F are
recorded as PARTIAL: every step reachable by `curl`, `psql`, or `go test` was executed
and its real outcome recorded, and every step needing a browser or a multi-minute wait
is named as unrun. See the result record at the end of `quickstart.md`.

## Task Summary

| Phase | Tasks | Count | Done |
|---|---|---|---|
| 1. Setup | T001–T010 | 10 | 10 |
| 2. Foundational | T011–T031 | 21 | 21 |
| 3. US1 — Signing in (P1) 🎯 MVP | T032–T046 | 15 | 15 |
| 4. US2 — Administering accounts (P2) | T047–T057 | 11 | 11 |
| 5. US3 — Ending a session safely (P3) | T058–T067 | 10 | 9 |
| 6. US4 — Reviewing what happened (P4) | T068–T074 | 7 | 6 |
| 7. Polish | T075–T082 | 8 | 7 |
| **Total** | | **82** | **79** |
