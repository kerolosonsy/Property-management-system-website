# Phase 1 Data Model: Project Foundation & Login

**Feature**: `001-project-setup-login` | **Date**: 2026-08-29 | **Store**: PostgreSQL 17

Four tables and two enumerated types. Every timestamp is `timestamptz` held in UTC and
rendered in Africa/Cairo (Constitution IV, FR-047). Every relationship is a real foreign
key. Migrations are forward-only under `api/migrations/`.

## Enumerated types

**`account_role`** — `admin`, `manager`. Exactly two values (FR-002). Adding a third is
a specification change, not a data change.

**`audit_action`** — `sign_in_succeeded`, `sign_in_failed`, `sign_out`,
`password_changed`, `password_reset`, `account_created`, `account_role_changed`,
`account_activated`, `account_deactivated`, `sessions_invalidated`,
`admin_recovery_used`. Covers every event FR-022 requires, plus the offline recovery
utility named in the plan's risk table.

## `account`

One row per person who may sign in.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key, `gen_random_uuid()` |
| `username` | `text` | Stored exactly as the administrator typed it (FR-039) |
| `username_canonical` | `text` | Derived per research §7. **Unique.** Matching and uniqueness both use this column |
| `display_name` | `text` | Arabic name shown on screen |
| `password_hash` | `text` | Argon2id encoded form, carrying its own salt and parameters (research §4) |
| `role` | `account_role` | Exactly one (FR-002) |
| `is_active` | `boolean` | Default `true`. Deactivation never deletes a row |
| `must_change_password` | `boolean` | Default `true`. Set on creation and on administrator reset (FR-010) |
| `created_at` | `timestamptz` | |
| `updated_at` | `timestamptz` | |
| `created_by` | `uuid` | References `account(id)`. Null only for the account created at installation |

**Constraints**

- `UNIQUE (username_canonical)` — the sole enforcement of FR-001 uniqueness. Uniqueness
  is never checked by reading first: two administrators creating the same username at
  the same moment must produce one success and one clean refusal.
- `CHECK (char_length(username) BETWEEN 3 AND 32)` — FR-036.
- Character-set validation for FR-036 is enforced in the **application layer**
  (`identity.AllowedUsernameRune`), not by a check constraint. A
  `CHECK (username ~ '^[\p{L}\p{Nd}._-]+$')` was specified originally and is wrong:
  PostgreSQL's POSIX regex engine accepts it at `CREATE TABLE` time but raises
  `invalid regular expression: invalid escape \ sequence` when the constraint is
  *evaluated*, because it does not implement the Perl Unicode property classes `\p{L}`
  and `\p{Nd}`. The effect is that no row can ever be inserted. Migration `0008` drops
  it. The application also rejects the invisible and bidirectional control characters
  listed in FR-037, so the refusal message is specific — which a regex constraint could
  not have produced anyway.
- `CHECK (char_length(display_name) BETWEEN 1 AND 120)`.
- `FOREIGN KEY (created_by) REFERENCES account(id) ON DELETE RESTRICT` — accounts are
  never deleted, so this never fires; it exists to make that intent structural.

**Rule not expressible as a constraint**: the system must never be left with no active
administrator (spec Edge Cases). Deactivation and demotion each run inside a transaction
that first takes a row lock over active administrators, counts them, and refuses if the
change would bring the count to zero. A check constraint cannot see other rows, and a
trigger enforcing it would duplicate logic the handler already needs for its Arabic
refusal message.

**State transitions**

```text
created (is_active, must_change_password)
   │  first sign-in → forced password change
   ▼
active ──────── administrator deactivates ───────▶ inactive
   ▲                                                  │
   └──────────── administrator reactivates ───────────┘

active ──── administrator resets password ────▶ active, must_change_password again
active ──── administrator changes role ───────▶ active, new role effective next request
```

Deactivation, password change, and password reset each revoke every session belonging to
the account in the same transaction (FR-020). A role change does **not** revoke sessions
— the new role is read on the next request (FR-034, FR-035).

## `session`

One row per active sign-in. Carries identity only; it is never the authority on role.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` | Primary key |
| `account_id` | `uuid` | References `account(id)`, `ON DELETE RESTRICT` |
| `token_sha256` | `bytea` | **Unique.** SHA-256 of the opaque token. The token itself is never stored (research §5) |
| `created_at` | `timestamptz` | Absolute lifetime is measured from here (FR-018) |
| `last_seen_at` | `timestamptz` | Idle lifetime is measured from here (FR-017) |
| `revoked_at` | `timestamptz` | Null while live. Set by sign-out and by FR-020 revocation |

A session is usable when `revoked_at IS NULL`, `last_seen_at > now() - 30 minutes`, and
`created_at > now() - 8 hours`. Expiry is therefore derived, not stored: a clock change
or a missed cleanup run can never leave a stale session usable. Rows past both limits are
deleted by a sweep; deletion is a housekeeping act, not the mechanism of expiry.

**Indexes**: unique on `token_sha256` (the lookup on every request); `account_id` (bulk
revocation); `last_seen_at` (the sweep).

## `login_failure`

Drives the progressive delay (FR-014, FR-015, FR-016, FR-040, FR-042). Deliberately not
linked to `account`.

| Column | Type | Notes |
|---|---|---|
| `username_canonical` | `text` | Primary key. The **submitted** username, canonicalized |
| `consecutive_failures` | `integer` | Reset to zero — by deleting the row — on success |
| `last_failure_at` | `timestamptz` | The wait is measured from here |

There is no foreign key to `account`, and that is the point: FR-040 requires a username
that does not exist to accumulate delay exactly as a real one does. A foreign key would
make the table's contents reveal which usernames are real.

Required wait: `min(60, 2^(consecutive_failures - 1))` seconds after `last_failure_at`,
so 1, 2, 4, 8, 16, 32, 60, 60… A row untouched for 15 minutes is deleted (FR-042).

## `audit_log`

Append-only. One row per recorded action (FR-022 through FR-025).

| Column | Type | Notes |
|---|---|---|
| `id` | `bigserial` | Primary key, also the tie-breaker for ordering within a timestamp |
| `occurred_at` | `timestamptz` | FR-023 |
| `action` | `audit_action` | |
| `actor_account_id` | `uuid` | References `account(id)`. Null when the actor was never authenticated — a failed sign-in with an unknown username |
| `actor_username_snapshot` | `text` | What was typed or who acted, as named at that moment |
| `actor_role` | `account_role` | Null when unauthenticated |
| `target_account_id` | `uuid` | References `account(id)`. Null when the action affected no account |
| `target_username_snapshot` | `text` | The affected account as named at that moment |
| `source_ip` | `inet` | FR-023 |
| `detail` | `jsonb` | Field names and identifiers only. Never a password, never a decrypted value (FR-025) |

**Why the snapshot columns exist**: a record must still read correctly after the account
it names is renamed or deactivated (spec Edge Cases). The foreign keys let the records
view filter by account; the snapshots let it display what was true when the action
happened. Records are never rewritten.

**Indexes**, one per filter FR-044 requires: `(occurred_at DESC, id DESC)` for the
default newest-first page, plus `actor_account_id`, `action`, and `target_account_id`.

**Privileges** (Constitution VIII, FR-024). Two database roles:

| Role | Business tables | `audit_log` |
|---|---|---|
| owner — migrations and the offline utility only | all | all |
| application — the running service | `SELECT, INSERT, UPDATE, DELETE` | `SELECT, INSERT` only |

`UPDATE` and `DELETE` on `audit_log` are never granted to the application role, so no
code path in the running service can alter a record, administrator included. The owner
credentials are not present in the service's environment; that residual power is stated
in `quickstart.md` so it is a known property.

Every audit row is written by the same transaction as the change it records. If the
audit insert fails, the change rolls back.

## Requirement coverage

| Requirement | Where it lives |
|---|---|
| FR-001, FR-038 | `account.username_canonical` unique index |
| FR-002 | `account_role` enum, `account.role` |
| FR-008 | `account.password_hash` — Argon2id, no reversible form stored |
| FR-010 | `account.must_change_password` |
| FR-014–FR-016, FR-040–FR-042 | `login_failure` |
| FR-017, FR-018, FR-019, FR-020 | `session` timestamps and `revoked_at` |
| FR-022–FR-025, FR-043–FR-048 | `audit_log` and its privileges |
| FR-034, FR-035 | Role read from `account` per request; sessions untouched by role change |
| FR-036, FR-037, FR-039 | `account` check constraints plus application-layer rejection |

No table in this feature stores money, an attachment, or a national identity number, so
Constitution IV's monetary rule and Constitution VII's envelope and column encryption are
not exercised here.
