# Phase 0 Research: Project Foundation & Login

**Feature**: `001-project-setup-login` | **Date**: 2026-08-29

Every decision below was reached against Constitution v1.1.0 and the 48 functional
requirements in [spec.md](./spec.md). Where the constitution already fixes a choice
(Angular, Go, PostgreSQL, REST/JSON, OpenAPI, Argon2id, TLS), only the open detail is
researched.

## 1. HTTP routing in the Go service

**Decision**: The standard library's `net/http.ServeMux`, using method-and-pattern
routes (`mux.HandleFunc("POST /api/v1/auth/login", …)`), with the route set generated
from the OpenAPI document by `oapi-codegen`'s `std-http-server` target.

**Rationale**: Since Go 1.22 the standard mux does method matching and path wildcards,
which is the entire reason projects used to reach for a router. The API has 11
operations. Adding a routing dependency would buy nothing this feature needs, and
Principle VI forbids a dependency the feature does not need. Go 1.27 is installed, so
the capability is present.

**Alternatives considered**: `chi` — a good router, but its selling points (middleware
composition across deep route trees, sub-routers) are unused at 11 operations. `gin`,
`echo` — full frameworks bringing their own context type, binding, and error
conventions, which would compete with the generated OpenAPI types.

## 2. PostgreSQL access

**Decision**: `jackc/pgx/v5` used directly through its pool, with hand-written SQL in
each `internal` package. No ORM, no query builder, no `sqlc` code generation.

**Rationale**: The feature has four tables and roughly twenty queries. `pgx` gives
native `timestamptz` handling, proper prepared statements, and real transaction
control, which Principle VIII needs because the audit row and the change it records
must commit together. `sqlc` is attractive at scale but adds a second code-generation
step and a schema-drift failure mode for twenty queries.

**Alternatives considered**: `database/sql` with `lib/pq` — an older driver in
maintenance mode, weaker type handling. GORM — hides the transaction boundary that
Principle VIII depends on being explicit. `sqlc` — revisit if the query count passes
roughly a hundred.

## 3. Schema migrations

**Decision**: `pressly/goose` with plain `.sql` migration files under
`api/migrations/`, applied by a command documented in `quickstart.md` and run inside
the database container's network.

**Rationale**: Constitution IV requires versioned, forward-only migrations committed
with the feature, and prohibits editing an applied migration. Goose files are ordinary
SQL, readable by anyone who knows the database and not only by a Go developer, and its
version table makes "which migrations have run" answerable directly.

**Alternatives considered**: `golang-migrate` — equally capable; goose was chosen for
the simpler embedding story and because its up/down files are one file rather than two.
Hand-rolled migration runner — rejected as reinventing a solved problem.

## 4. Argon2id parameters

**Decision**: `golang.org/x/crypto/argon2`, variant `argon2id`, with time = 3,
memory = 64 MiB, parallelism = 2, a 16-byte random salt per password, and a 32-byte
derived key. The salt and parameters are stored alongside the hash in the standard
encoded string form, so parameters can be raised later without invalidating existing
passwords.

**Rationale**: 64 MiB with three passes sits comfortably above current minimum
guidance while keeping a sign-in near 200 ms on the target hardware, which SC-001's
15-second human budget absorbs without noticing. Encoding the parameters in the stored
value means a future increase re-hashes each password on the owner's next successful
sign-in rather than forcing a reset.

**Alternatives considered**: bcrypt — capped work factor and a 72-byte input limit,
awkward against a 12-character-minimum policy with Arabic passwords where a single
character can occupy several bytes. scrypt — acceptable, but Argon2id is the
constitution's named choice. Raising memory to 256 MiB — rejected as it would make
several concurrent sign-ins contend for memory on a developer laptop for no meaningful
security gain at this threat level.

## 5. Session mechanism

**Decision**: An opaque 256-bit random token generated with `crypto/rand`, delivered in
a cookie marked `HttpOnly`, `Secure`, `SameSite=Strict`, and `Path=/`. The server
stores only the SHA-256 of the token, alongside the account it belongs to, its creation
time, and its last-activity time. On every request the server hashes the presented
token, looks up the session, and joins to the account to read the current role and
active state.

**Rationale**: FR-034 forbids trusting a role recorded at sign-in. A signed token
carrying claims (JWT) fails that requirement by construction — the claims are exactly a
snapshot of sign-in time — and would need a revocation list to satisfy FR-020, at which
point the stateless advantage is gone and a database lookup happens anyway. Storing the
hash rather than the token means a leaked database dump cannot be used to impersonate a
live session. Idle and absolute expiry (FR-017, FR-018) are two timestamp comparisons
on a row that is already being read.

**Alternatives considered**: JWT in a cookie — rejected against FR-034 and FR-020.
JWT in memory with a refresh token — same objection, plus a session that dies on page
refresh. Server-side session store in Redis — a second data store for four tables'
worth of state.

## 6. Progressive delay

**Decision**: A `login_failure` table keyed by the canonical form of the *submitted*
username, holding a consecutive-failure count and the time of the last failure. The
required wait is `min(60, 2^(count-1))` seconds measured from the last failure: 0
before any failure, then 1, 2, 4, 8, 16, 32, 60, 60… A sign-in submitted before that
wait has elapsed is refused with the remaining seconds. A successful sign-in deletes
the row. Rows untouched for 15 minutes are deleted by a sweep that runs on write.

**Rationale**: FR-040 requires the delay to behave identically for usernames that do
not exist, which is only possible if the counter is keyed by the submitted string
rather than by an account. Keying on the canonical form stops an attacker resetting
their own counter by varying letter case or an alef variant. Storing the counter in
PostgreSQL rather than in process memory means a restart cannot be used to clear it,
and the state is visible when diagnosing a complaint.

**Rejected refinement**: sleeping inside the request for the remaining time instead of
refusing immediately. It holds a connection open per attacker request, which turns the
defence into a resource-exhaustion vector, and it prevents FR-015's "tell the user how
much of the wait remains".

**Unbounded-growth note**: because unknown usernames create rows, the table is
attacker-writable in row count. The 15-minute sweep plus a hard cap on stored username
length bounds it; at this deployment's scale the cost of an attacker inserting rows is
already dominated by the delay they are subject to.

## 7. Arabic username canonicalization

**Decision**: Each account stores `username` exactly as typed and `username_canonical`
derived from it, with a unique index on the canonical column. The derivation, in order:
Unicode NFKC normalization (`golang.org/x/text/unicode/norm`); Eastern Arabic digits
mapped to Western; removal of the Arabic diacritics U+064B–U+0652 and the tatweel
U+0640; unification of alef forms (أ, إ, آ, ٱ → ا), of alef maqsura (ى → ي), and of
ta marbuta (ة → ه); then Unicode case folding for the Latin portion. Sign-in looks up
by canonical form; uniqueness is enforced by the index, not by a prior existence check.

**Rationale**: FR-038 names exactly these equivalences. Doing the work in the database
as a generated column was considered, but the mapping table is long and belongs where
it can be unit-tested — this and the delay schedule are the two pieces the constitution's
manual-verification stance cannot cover by looking at a screen, so both get automated
tests. Enforcing uniqueness with the index rather than a check-then-insert removes the
race where two administrators create the same username simultaneously.

**Rejected characters**: FR-037 requires refusing zero-width and bidirectional control
characters. The rejected set is U+200B–U+200F, U+202A–U+202E, U+2066–U+2069, and
U+FEFF. These are rejected with an explanatory Arabic message rather than stripped,
because silently changing what someone typed into their own username is worse than
telling them.

## 8. Where Eastern digits are converted

**Decision**: In both places. An Angular directive converts as the user types, which is
what FR-033 means by "visible to the user as they enter the value". The Go service
converts again on receipt, before validation and before storage.

**Rationale**: The client-side conversion is a user-experience requirement; the
server-side conversion is a correctness requirement, because the API is reachable
without the browser and FR-032 says no Eastern digit is ever stored. Neither one alone
satisfies both requirements. The conversion touches digit code points only and leaves
surrounding Arabic letters untouched.

## 9. Making the audit log append-only

**Decision**: Two database roles. The owner role runs migrations and is used by nothing
else. The application connects as a second role holding `SELECT, INSERT, UPDATE,
DELETE` on the business tables but only `SELECT, INSERT` on `audit_log`, with `UPDATE`
and `DELETE` never granted. The audit row is written by the same `pgx` transaction as
the change it records, so a failed audit write rolls the change back.

**Rationale**: Constitution VIII requires the restriction to be enforced by database
privileges rather than by convention, and explicitly extends it to administrators. A
trigger raising an exception was considered as an additional layer, but a trigger and a
privilege that enforce the same rule is two mechanisms to keep in step; the privilege is
the one the constitution names, and unlike a trigger it cannot be disabled by the role
it constrains.

**Consequence to accept**: the owner credentials can still alter audit rows. They exist
only in the migration step and the offline administrator utility, never in the running
service's environment. This is stated in `quickstart.md` so it is a known property
rather than a discovered one.

## 10. Local runtime: TLS, database, and code generation

**TLS**: A self-signed certificate for `localhost` generated with the installed OpenSSL
3.6.3 into `infra/certs/`, which is gitignored. The Go server serves HTTPS only, and
the Angular development server is configured with the same certificate so the browser
holds one exception rather than two. `mkcert` is not installed and is not required; the
one-time browser warning is documented in `quickstart.md` as an expected outcome.

**Database**: PostgreSQL 17 via `infra/docker-compose.yml`. Docker 29.5.2 is installed;
there is no `psql` on the host, so every database command in the documentation runs
through `docker compose exec`.

**Code generation**: `oapi-codegen` produces Go request and response types plus a
server interface from `contracts/openapi.yaml`; the compiler then fails if a handler
drifts from the contract, which is what makes Principle III enforceable rather than
aspirational. `@openapitools/openapi-generator-cli` with the `typescript-angular`
generator produces the Angular client from the same document; Java 23 is installed and
satisfies its runtime requirement. Both outputs land in directories named `gen`/`api`
and are regenerated, never edited.

**Angular version**: not pinned in this document. It is pinned by the scaffolding task,
which records the exact version here and commits the lockfile in the same change, so
the plan never claims a version that was not actually installed.

Pinned at scaffold time (2026-08-29): **Angular 22.1.x** (`@angular/core ^22.1.0`, `@angular/cli ^22.1.6`, `@angular/build ^22.1.6`, `typescript ~6.0.2`). Lockfile committed alongside. The generated client uses `@openapitools/openapi-generator-cli` with the `typescript-angular` generator (`ngVersion=22`, `useSingleRequestParameter=true`, `apiModulePrefix=Pms`). The dev server proxies `/api/v1` to `https://localhost:8443` so the HttpOnly session cookie is sent on same-origin requests; in production the same path is expected to be served by a reverse proxy on the API origin.

## 11. Post-design Constitution re-check

Re-evaluated after `data-model.md`, `contracts/openapi.yaml`, and `quickstart.md` were
written. All eight gates still pass.

- **I** — Every operation in the contract declares its required role, and the design
  reads role per request (§5). No design element trusts a client value.
- **II** — Arabic RTL applies to the three screens; digits are covered by §8.
- **III** — The contract exists and generates both sides (§10); no handler is designed
  outside it.
- **IV** — Four tables, every relationship a foreign key, forward-only migrations (§3),
  all timestamps `timestamptz`. No money in this feature.
- **V** — `quickstart.md` carries the full procedure including manager-denied steps.
- **VI** — No dependency was added that the feature does not use: no router (§1), no
  ORM (§2), no second data store (§5), no trigger duplicating a privilege (§9).
- **VII** — TLS only (§10), Argon2id (§4), hashed session tokens and hardened cookies
  (§5), secrets from the environment. No attachment or encrypted column is exercised
  by this feature; the key variable is established so the first upload feature inherits
  it.
- **VIII** — Audit rows written in the same transaction, application role granted
  `INSERT` and `SELECT` only (§9), and now readable through an administrator-only,
  filterable view.

No violation arose, so `plan.md`'s Complexity Tracking table stays empty.
