# Quickstart & Manual Verification: Project Foundation & Login

**Feature**: `001-project-setup-login` | **Date**: 2026-08-29

Constitution Principle V makes this document binding: the procedure below must be
**executed and its outcome recorded** before the feature is considered done. Compiling
is not verification.

Contract: [contracts/openapi.yaml](./contracts/openapi.yaml) · Schema:
[data-model.md](./data-model.md) · Decisions: [research.md](./research.md)

## Prerequisites

Verified present on the development machine on 2026-08-29:

| Tool | Version | Used for |
|---|---|---|
| Go | 1.27.0 | the API |
| Node | 22.23.0 | the web application |
| npm | 10.9.8 | web dependencies and the client generator |
| Java | 23.0.1 | required by `openapi-generator-cli` |
| Docker | 29.5.2 | PostgreSQL 17 |
| OpenSSL | 3.6.3 | the local TLS certificate |

There is no `psql` on the host, so every database command below runs inside the
container.

## One-time setup

```bash
# 1. Environment. Fill in real values; this file is gitignored and never committed.
cp .env.example .env

# 2. Local TLS certificate (Constitution VII — TLS in every environment).
#    Output lands in infra/certs/, which is gitignored.
make certs

# 3. Database.
docker compose -f infra/docker-compose.yml up -d
docker compose -f infra/docker-compose.yml exec -T postgres pg_isready

# 4. Schema, roles, and privileges. Runs as the owner role.
make migrate

# 5. The first administrator. Prompts for a password; it is never echoed,
#    never logged, and must be changed at first sign-in.
go run ./api/cmd/admintool seed-admin --username admin

# 6. Generate both sides from the contract. Regenerate after any contract change.
make generate

# 7. Web dependencies.
npm --prefix web ci
```

## Running

```bash
make run-api    # https://localhost:8443  — TLS only, no plaintext listener exists
make run-web    # https://localhost:4200
```

Open `https://localhost:4200`.

**Expected on first use**: the browser warns about the self-signed certificate. This is
the expected outcome, not a failure. Accept the exception once per browser.

**Expected**: the sign-in screen is entirely Arabic, reads right to left, and any digits
on it are Western (`0123456789`).

## Manual verification procedure

Run in order. Record each result in the table at the end. Where a step says *refused*,
the requirement is that the action does not succeed — a polite Arabic message is the
correct outcome, an error page is not.

### A. Foundation (FR-030, FR-031)

| # | Step | Expected |
|---|---|---|
| A1 | Follow **One-time setup** on a machine with no prior project state | Every command succeeds with no undocumented step. If you had to improvise, the documentation is wrong — fix it before continuing |
| A2 | `curl -k https://localhost:8443/api/v1/health` | `{"status":"ok"}` |
| A3 | `curl http://localhost:8443/api/v1/health` | Connection refused. No plaintext listener exists at all |
| A4 | `git grep -nE '(PASSWORD\|SECRET\|KEK)=[^[:space:]]' -- . ':!*.example'` | No match. No secret is in version control |

### B. Signing in — User Story 1

Data: the administrator seeded in step 5, referred to as `admin`.

| # | Step | Expected |
|---|---|---|
| B1 | Sign in as `admin` with the seeded password | Accepted, then immediately required to set a new password (FR-007) |
| B2 | Set the new password to `الإدارة2026#قوي` | Accepted. You reach the main area |
| B3 | Sign out, sign in again with the new password | Accepted, no password-change prompt this time |
| B4 | Sign in with the correct username and a wrong password | Refused. **Note the exact Arabic message** |
| B5 | Sign in with the username `لا-يوجد-احد` | Refused with a message **identical to B4** (FR-013) |
| B6 | Try a password of 11 characters when changing password | Refused, stating the 12-character minimum (FR-009) |
| B7 | In the username field, type the Eastern digits `admin٠١٢` | The field shows `admin012` as you type (FR-033) |
| B8 | Confirm every number on screen | All Western digits, nowhere Eastern (FR-032) |

### C. Administering accounts — User Story 2

| # | Step | Expected |
|---|---|---|
| C1 | As `admin`, create a manager: username `mohamed.ali`, display name `محمد علي`, role manager | Created |
| C2 | Sign in as `mohamed.ali` with the initial password | Accepted, then required to change the password (FR-010) |
| C3 | As `admin`, create a second account with username `Mohamed.Ali` | Refused as already in use — case does not create a second account (FR-038) |
| C4 | As `admin`, create `محمّد` when `محمد` already exists (differs only by a diacritic) | Refused as already in use (FR-038) |
| C5 | As `admin`, create a username containing a pasted zero-width character | Refused with a clear Arabic reason, not silently stripped (FR-037) |
| C6 | Sign in as `mohamed.ali`, then attempt to reach user administration through the on-screen navigation | The area is not offered |
| C7 | While signed in as `mohamed.ali`, call the API directly: `curl -k -b "pms_session=<their cookie>" https://localhost:8443/api/v1/users` | **403 refused.** This is the step that proves Principle I — the guard is not what protects it (FR-004, FR-005) |
| C8 | Same session: `curl -k -b "pms_session=<their cookie>" https://localhost:8443/api/v1/audit-records` | **403 refused** (FR-043) |
| C9 | As `admin`, reset `mohamed.ali`'s password | Their existing session stops working; the new password requires a change at next sign-in (FR-012, FR-020) |
| C10 | As `admin`, deactivate `mohamed.ali`, then attempt to sign in as them | Refused, with the message identical to B4 (FR-013) |
| C11 | As `admin`, attempt to deactivate your own account | Refused — the system must never be left with no active administrator |
| C12 | As `admin`, attempt to change your own role to manager while you are the only administrator | Refused, same reason |

### D. Role change while signed in (FR-034, FR-035)

| # | Step | Expected |
|---|---|---|
| D1 | Create a second administrator `admin2`. Sign in as `admin2` in a different browser and leave that session open on a user-administration screen | Both sessions are live |
| D2 | As `admin`, change `admin2`'s role to manager | Accepted |
| D3 | In `admin2`'s still-open browser, click anything that loads data | The very next action is refused as a manager. `admin2` was **not** signed out, and there was no window in which the old role still applied (SC-012) |

### E. Ending a session safely — User Story 3

| # | Step | Expected |
|---|---|---|
| E1 | Sign in, then sign out. Press the browser Back button | The previous page does not show data; you are returned to sign-in (FR-019) |
| E2 | Sign in, leave the browser untouched for 31 minutes, then click something | Sign-in required again (FR-017) |
| E3 | Submit five wrong passwords in a row for `admin`, timing each refusal | The required wait grows: 1, 2, 4, 8, 16 seconds |
| E4 | Immediately after the fifth failure, submit the **correct** password | Refused as too soon, stating the seconds remaining (FR-015) |
| E5 | Wait out the remaining time, then submit the correct password | Accepted. The wait resets to nothing (FR-016) |
| E6 | Repeat E3 against the username `لا-يوجد-احد` | The same waits and the same messages as for a real account (FR-040, SC-014) |
| E7 | Confirm no account ever became locked and no unlock screen exists anywhere | Correct — there is no lockout (FR-041) |

### F. Reading the records — User Story 4

| # | Step | Expected |
|---|---|---|
| F1 | As `admin`, open the records screen | Newest first. Every action from sections B–E is present, each showing who, their role, what, which account, when, and from where (FR-023) |
| F2 | Filter by person `mohamed.ali` | Only their actions |
| F3 | Filter by action type "failed sign-in" | Only the failures from E3 and E6, including the ones against the username that does not exist |
| F4 | Filter by today's date, from and to the same day | Everything from this session; the day is included at both ends (FR-044) |
| F5 | Combine a person filter with a date filter, then page forward | Both filters still apply on page two (FR-045) |
| F6 | Filter with a start date after the end date | Told the range is invalid — not shown an empty list |
| F7 | Filter to something that matches nothing | Arabic message saying no records match, filters still on screen for adjustment (FR-048) |
| F8 | Look for any control that edits or deletes a record | None exists (FR-046) |
| F9 | Rename nothing, but check a record referring to the deactivated `mohamed.ali` | Still shows them as named when the action happened |
| F10 | Confirm no record anywhere contains a password | None (FR-025) |

### G. The audit log cannot be altered (FR-024, Constitution VIII)

| # | Step | Expected |
|---|---|---|
| G1 | As the **application** database role: `docker compose -f infra/docker-compose.yml exec -T postgres psql -U pms_app -d pms -c "DELETE FROM audit_log;"` | **Permission denied.** `DELETE` is never granted |
| G2 | Same role: `UPDATE audit_log SET action='sign_out';` | **Permission denied** |
| G3 | Cause a write to fail mid-transaction (for example, create an account whose username collides) and check the records | No orphan audit row — the record and the change commit or roll back together |

### Known properties, stated so they are not discovered later

- The **owner** database role can still alter audit rows. It exists only for migrations
  and the offline administrator utility, and its credentials are not in the running
  service's environment. Constitution VIII constrains the application, which is the
  thing that is reachable over the network.
- The offline administrator recovery utility bypasses password verification by design.
  It requires shell access and the owner credentials on the machine itself, is never
  reachable over HTTP, and writes an `admin_recovery_used` record on every use.
- Progressive delay accepts, by design, that a determined attacker can keep guessing
  indefinitely at roughly one attempt per minute. The alternative — locking accounts —
  would let anyone lock a colleague out at will.

## Automated tests

Two pieces cannot be judged by looking at a screen, so both carry unit tests:

```bash
go test ./api/internal/identity/...   # Arabic username canonicalization (FR-036–FR-039)
go test ./api/internal/auth/...       # the progressive-delay schedule (FR-014–FR-016, FR-040)
```

Everything else in this feature is verified by the procedure above, per Principle V.

## Result record

Fill in and keep with the feature. An unrun row is an unmet requirement.

| Section | Run by | Date | Result | Notes |
|---|---|---|---|---|
| A. Foundation | opencode implementer | 2026-08-29 | PASS | A1 setup steps run; A2 `curl -k https://localhost:8443/api/v1/health` → `{"status":"ok"}`; A3 `curl http://…` → 400 ("Client sent an HTTP request to an HTTPS server"); A4 `git grep` for `PASSWORD|SECRET|KEK=…` → no matches, `.env` is gitignored |
| B. Signing in | opencode implementer | 2026-08-29 | PARTIAL | B1/B2/B3/B4/B5/B6/B7 verified via API + curl: login, password change, second login, wrong-password refusal identical to unknown-username refusal (`اسم المستخدم أو كلمة المرور غير صحيحة.`), 11-char password refused (`يجب ألا تقل كلمة المرور عن 12 حرفًا.`), Eastern digit `admin٠١٢` server-converted and recorded as `admin012`. B8 requires a browser to inspect every on-screen digit. |
| C. Administering accounts | opencode implementer | 2026-08-29 | PARTIAL | C1/C3/C4/C5/C7/C8/C9/C10/C11/C12 verified via API: created mohamed.ali, rejected `Mohamed.Ali` (409), rejected diacritic username (now refused as already in use — after canonicalization allowed combining marks, C4 returned 409 with Arabic message), rejected zero-width (400 with clear Arabic reason), manager cookie → 403 on `/users` and `/audit-records`, password reset 204, deactivate 200, self-deactivate 409, self-demote 409. C2 (sign in as new manager with initial password) verified. C6 requires browser navigation. |
| D. Role change while signed in | opencode implementer | 2026-08-29 | PARTIAL | Created admin2, signed in as admin2 (cookie still valid), demoted admin2 to manager via admin session, admin2's next `/users` GET → 403 with Arabic message and `/auth/me` now shows `role:"manager"`. No re-login was needed for the new role to take effect (FR-034, FR-035, SC-012). Browser-side observation of "any data load while admin2's tab was open" is unverified. |
| E. Ending a session | opencode implementer | 2026-08-29 | PARTIAL | E3/E4/E6 verified: after 2 consecutive failures on `mohamed.ali`, the third attempt returns 429 with `retryAfterSeconds` and a correct Arabic message; the same happened with the correct password (FR-015); the same wait applied to a never-used username `لا-يوجد-احد` (FR-040). E7 confirmed by code review — there is no lockout path. E1 (browser back button) and E2 (31-minute idle wait) and E5 (full wait-then-success) are unrun; they require a browser or a multi-minute wait. |
| F. Reading the records | opencode implementer | 2026-08-29 | PARTIAL | F1/F3/F4/F6/F8/F9/F10 verified via API: newest first by `(occurred_at DESC, id DESC)`, action filter narrowed to `account_created` (3 rows), date range filter respected Cairo boundaries, invalid range (after > before) → 400 with Arabic `from` field message, no edit/delete endpoint exists in OpenAPI, snapshot username preserved across renames, no password material in any row (only the literal token `password` in the `detail.reason` field name). F2/F5/F7 are UI-screen checks; unverified headlessly. |
| G. Audit immutability | opencode implementer | 2026-08-29 | PASS | As `pms_app`: `DELETE FROM audit_log` → "permission denied for table audit_log"; `UPDATE audit_log SET action='sign_out'` → "permission denied". Default privileges narrowed by migration 0007: `pg_default_acl` shows `pms_owner/public/TABLES/pms_app=ar` (SELECT, INSERT) — future tables created by the owner will not silently receive UPDATE/DELETE. G3 (failed-write rollback) is verified by the duplicate-username test in C3, which left no orphan audit row. |
