<!--
SYNC IMPACT REPORT
==================
Version change: 1.1.0 → 1.2.0
Bump rationale: MINOR — Principle VII is materially expanded to admit a second,
bounded route into the encrypted set: fields an administrator designates as
sensitive at runtime. No principle is removed, and no already-shipped behaviour
becomes non-compliant: the four named columns keep exactly the rule they had, and
the fixed set still moves only by amendment.

This bump was weighed against MAJOR. Delegating any part of the encrypted set to a
runtime decision does change who controls that set, which is close to a
redefinition. It is recorded as MINOR because the delegation is additive and
fenced on four sides — free-text fields only, custom fields only, fixed once
values exist, and stripped of search/sort/filter/report exactly as the built-in
encrypted columns are. A future amendment that widened administrator designation
to built-in columns, or that let a designation be revoked over existing data,
would be MAJOR.

Principles in this version:
  I.    Server-Enforced Authorization (NON-NEGOTIABLE)   [unchanged]
  II.   Arabic-First, RTL-Native Interface               [unchanged]
  III.  Contract-First API                               [unchanged]
  IV.   Relational Integrity & Money Safety              [unchanged]
  V.    Manual Verification Discipline                   [unchanged]
  VI.   Simplicity & Surgical Change                     [unchanged]
  VII.  Encryption & Data Confidentiality (NON-NEGOTIABLE)   [EXPANDED in 1.2.0]
  VIII. Auditability                                         [unchanged]

Sections modified:
  - Principle VII, "Database fields" — split into encryption fixed by this
    document and encryption designated by an administrator, with the four bounds
    on designation, and the exclusion of designated fields from search, sort,
    filter, and report.
  - Principle VII, rationale — extended to explain why the delegation exists and
    why it is fenced.
  - Development Workflow & Quality Gates, step 3 — a feature storing designated
    sensitive fields now requires an encryption task and a search-exclusion task.

Driven by: feature 002-properties-crud, whose administrator-defined custom fields
would otherwise have offered a plaintext, searchable home for exactly the four
identifiers this principle exists to protect.

Templates requiring updates:
  ✅ .specify/templates/plan-template.md — gate VII extended; version reference
     updated to 1.2.0.
  ✅ .specify/templates/spec-template.md — no change required.
  ✅ .specify/templates/tasks-template.md — no change required; the new task
     obligation lives in the Development Workflow section above.
  ✅ AGENTS.md — version reference updated to 1.2.0.
  ✅ CLAUDE.md — version reference updated to 1.2.0.

Deferred TODOs: none.
-->

# Property Management System Constitution

## Core Principles

### I. Server-Enforced Authorization (NON-NEGOTIABLE)

The system has exactly two roles: **admin** and **manager**.

- **admin** MAY perform every operation, including system configuration.
- **manager** MAY create, read, update, and delete operational data and generate
  reports. A manager MUST NOT be able to read or modify any configuration
  resource, by any route, under any circumstance.

Rules:

- Every HTTP handler MUST resolve the caller's role from a server-verified
  session/token and check permission before touching data. Authorization checks
  MUST NOT be delegated to middleware alone if the middleware cannot see the
  specific resource being accessed.
- The default MUST be deny. A route with no explicit permission rule MUST reject
  the request rather than allow it.
- Angular route guards, disabled buttons, and hidden menu items are **user
  experience only**. They carry zero security weight and MUST never be cited as
  the reason an endpoint is safe.
- Role MUST NOT be read from request body, query string, or any client-supplied
  header.

**Rationale**: This is a system of record for leases, tenants, and money. A
configuration change made by the wrong person is silent and expensive. Two roles
is a small enough surface that there is no excuse for an unchecked endpoint.

### II. Arabic-First, RTL-Native Interface

The user interface language is **Arabic only**. Layout direction is **RTL**.

- The application root MUST set `lang="ar"` and `dir="rtl"`.
- All user-visible strings MUST be Arabic. No English placeholder text ships to
  a screen a user can reach.
- Layout MUST use logical CSS properties (`margin-inline-start`,
  `padding-inline-end`, `inset-inline-*`, `text-align: start`) rather than
  physical `left`/`right` properties, so mirroring is structural and not patched
  per component.
- Imported Claude Design `.dc.html` sources are a **visual reference, not
  shippable markup**: they carry template placeholders and mock data, and their
  stylesheet may mix physical and logical CSS. Every plan that consumes one
  states what it takes from the design (tokens, component classes, layout) and
  audits the result against the logical-property rule above. Where a design's own
  CSS uses physical properties, the vendored copy converts them and says so.
  A design that already ships Arabic RTL copy is a starting point, not a licence
  to skip that audit.
- Icons and imagery that encode direction (arrows, progress, chevrons) MUST be
  mirrored. Icons that do not (logos, clocks, media controls) MUST NOT be.
- Numerals, dates, and currency MUST render consistently across the app; the
  chosen numeral system is decided once and applied globally, never per screen.

**Rationale**: RTL retrofitted after the fact costs more than RTL from line one,
and half-mirrored screens read as broken to the only users this product has.

### III. Contract-First API

The OpenAPI document in `contracts/` is the single source of truth for the
Angular ↔ Go boundary.

- An endpoint MUST exist in the OpenAPI spec before it is implemented.
- The Angular HTTP client MUST be generated from that spec. Hand-written
  request/response interfaces that duplicate spec types are prohibited.
- Any change to a request shape, response shape, status code, or error body is a
  spec change first and a code change second.
- Transport is REST over JSON. Errors MUST use one consistent error body shape
  across every endpoint.

**Rationale**: One spec file keeps two languages honest. Without it, the Angular
model and the Go struct drift, and the drift is only discovered by a user.

### IV. Relational Integrity & Money Safety

Storage is **PostgreSQL**. The database enforces correctness; the application
does not merely hope for it.

- Every relationship MUST be a real foreign key. Orphaned units, leases, or
  payments MUST be impossible at the schema level.
- Schema changes MUST ship as versioned, forward-only migration files committed
  with the feature. Editing an already-applied migration is prohibited.
- Monetary amounts MUST be stored as integer minor units (piastres) or
  `NUMERIC`. Floating-point types MUST NOT be used for money, ever.
- Currency is **EGP**. Dates are **Gregorian**. Timestamps MUST be stored in UTC
  (`timestamptz`) and rendered in Africa/Cairo.
- Any operation that writes more than one row across more than one table MUST run
  in a single transaction.

**Rationale**: Rent, deposits, and lease dates are the product. A rounding error
or a dangling foreign key here is not a bug report, it is a dispute with a
tenant.

### V. Manual Verification Discipline

This project does **not** enforce an automated test gate. That trade is
deliberate, and it comes with a binding obligation.

- Every feature MUST ship a written manual verification procedure in its
  `quickstart.md`: concrete steps, the exact data to enter, and the expected
  result for each step.
- Each procedure MUST include at least one step performed as a **manager** that
  confirms a configuration action is refused, whenever the feature touches
  configuration.
- The procedure MUST be executed, and its outcome recorded, before the feature is
  considered done. "It compiles" is not verification.
- Automated tests are permitted and welcome where logic is genuinely tricky
  (rent proration, date math, report aggregation). They are never required.

**Rationale**: Skipping automated tests is affordable only if verification still
happens somewhere. Writing the steps down is what keeps "minimal testing" from
degrading into "untested".

### VI. Simplicity & Surgical Change

- Build the minimum that satisfies the specification. No speculative
  abstractions, no configurability that was not asked for, no error handling for
  scenarios that cannot occur.
- Do not introduce a framework, library, or architectural layer that the current
  feature does not need.
- When editing existing code, change only what the feature requires. Do not
  reformat, rename, or "improve" adjacent code in the same change.
- Every changed line MUST trace to a requirement in the feature's specification.

**Rationale**: This is a two-role system with local-only deployment. Complexity
added now is complexity carried forever, unpaid for by any user.

### VII. Encryption & Data Confidentiality (NON-NEGOTIABLE)

**Attachments** — every uploaded file (lease scan, national ID image,
maintenance photo, report export) MUST be encrypted at rest. No exceptions, no
"temporary" plaintext.

- Files MUST be encrypted with **AES-256-GCM** using **envelope encryption**:
  each file gets a fresh random 256-bit data key (DEK), and that DEK is stored
  only in its key-wrapped form, encrypted by the master key (KEK).
- The plaintext bytes MUST NOT touch disk at any point. Encryption happens in the
  request path, streaming; a plaintext temp file is a violation even if deleted
  afterwards.
- The database stores file metadata, the wrapped DEK, and the nonce. It MUST NOT
  store the plaintext DEK, the KEK, or the file bytes.
- The KEK MUST come from an environment variable. It MUST NOT be committed,
  logged, written to the database, or included in an error message. Key rotation
  re-wraps DEKs; it does not require re-encrypting files.
- Files on disk MUST be named by opaque identifier. The original filename is
  metadata only, and MUST NOT determine the storage path.
- Attachments MUST be served only through an authorized handler that decrypts per
  request after a Principle I role check. No static file server, no directly
  reachable upload directory, no signed-path shortcut.
- The GCM authentication tag MUST be verified on every read. A verification
  failure is a hard error; partial or unauthenticated content MUST NOT be
  returned.

**Database fields** — encryption at the column and value level applies to identity
and financial identifiers, and to fields an administrator designates:

- **Encrypted by this document (the fixed set)**: national ID, passport number,
  bank account number, IBAN. Adding a built-in column to this set is a
  **constitution amendment**, not an implementation detail, because it removes
  that column from every search, sort, filter, and report.
- **Encrypted by administrator designation**: a free-text custom field that an
  administrator marks sensitive. This is the only encryption decision this
  document delegates to runtime, and it is bounded on four sides:
  - Only **free-text** fields MAY be designated. A field whose value comes from a
    published list of choices MUST NOT be, because encrypting a value an attacker
    can enumerate from that list conceals nothing.
  - Only **administrator-defined custom fields** MAY be designated. Built-in
    columns move into or out of the fixed set by amendment only.
  - Designation is **fixed at definition time** and MUST NOT change in either
    direction once any record holds a value for that field.
  - A designated field carries the **same consequence** as the fixed set: it MUST
    be excluded from search, sort, filter, and report, and the interface MUST say
    so in Arabic rather than silently omitting it.
- A designated field's values MUST use the AES-256-GCM envelope scheme required
  for attachments above: the KEK comes from the environment and MUST NOT encrypt
  a value directly, the wrapped data key and nonce are stored alongside the
  ciphertext, and the authentication tag MUST be verified on every read. The
  granularity of the data key is a design decision for the feature's plan.
- Any screen that defines custom fields MUST state that national ID, passport,
  bank account, and IBAN values belong in a designated field. An administrator who
  records one of those four in an undesignated field violates this principle; the
  system states the rule, and the audit trail records who defined the field.
- **Plaintext, deliberately**: names, phone numbers, email addresses, physical
  addresses — because managers must search and report on them, and Principle IV
  reports must remain possible.
- Decryption happens only inside a handler that has already passed a role check.
  Decrypted values MUST NOT appear in logs, error messages, or audit records.

**Transport and credentials**:

- **TLS is required in every environment, including local development.**
  Self-signed certificates are acceptable locally. A plaintext HTTP listener MUST
  NOT be exposed, not even as a redirect-only convenience.
- Passwords MUST be hashed with **Argon2id**. Plain hashes (MD5, SHA-*),
  unsalted hashes, and reversible encryption of passwords are prohibited.
- Session tokens MUST be delivered in cookies marked `HttpOnly`, `Secure`, and
  `SameSite`. Tokens MUST NOT be readable by JavaScript or placed in URLs.
- All secrets — KEK, database credentials, session signing keys — come from
  environment variables and are never committed, not even a development value.
- No secret, key, token, password, decrypted sensitive field, or file content
  MUST ever be written to a log or returned in an error body.

**Rationale**: The system holds national IDs and lease documents for real people.
"Encrypted" that only means full-disk encryption protects against a stolen
laptop and nothing else. Envelope encryption with the key outside the database
means a leaked dump is inert. The plaintext exceptions are named explicitly so
the trade is visible rather than discovered later at a broken report screen.

Administrator designation exists because a system that lets administrators invent
their own fields cannot enumerate in advance what they will put in them. Refusing
to encrypt anything an administrator defines would hand them a plaintext,
searchable home for exactly the four identifiers this principle protects; encrypting
everything they define would break the search those fields are for. Designation
puts the choice where the knowledge is, and the four bounds keep it from becoming a
general licence to move the encryption line at runtime.

### VIII. Auditability

Every state change and every attachment access MUST leave a record.

- The audit log MUST record: actor identity, actor role, action, entity type,
  entity identifier, UTC timestamp, and source IP.
- **Writes**: every create, update, and delete on a business record (property,
  unit, tenant, lease, payment, maintenance request, configuration) MUST be
  logged.
- **Attachment access**: every download or decryption of an attachment MUST be
  logged, including who and when. Reads of ordinary records need not be logged.
- The audit table is **append-only**. No application code path may update or
  delete an audit row, and that restriction MUST be enforced by database
  privileges, not by convention. This applies to admin as well.
- The audit write MUST occur in the same transaction as the change it records. If
  the audit write fails, the change MUST roll back.
- Audit rows MUST NOT contain decrypted sensitive values, file contents, or
  secrets — identifiers and field names only.

**Rationale**: "Who deleted this tenant" and "who downloaded that ID scan" are
questions that get asked exactly once, long after the fact, and cannot be
answered retroactively. Making the log append-only at the database level is what
separates an audit trail from a table someone can quietly edit.

## Technology & Architecture Constraints

**Stack** (fixed; changing any entry is a MAJOR amendment):

| Layer      | Choice                                              |
|------------|-----------------------------------------------------|
| Frontend   | Angular (TypeScript), Arabic-only, RTL              |
| Backend    | Go                                                  |
| Transport  | REST + JSON over TLS, described by OpenAPI          |
| Database   | PostgreSQL                                          |
| Crypto     | AES-256-GCM (envelope) for files and encrypted columns; Argon2id for passwords |
| Deployment | Local development only                              |

**Repository layout** (monorepo):

```text
web/         # Angular application
api/         # Go service
contracts/   # OpenAPI specification — source of truth for the API
specs/       # Spec Kit feature specifications, plans, and tasks
```

**Key and secret handling**:

- The master key (KEK), database credentials, and session signing key are
  supplied as environment variables. A committed `.env.example` documents the
  variable names and MUST contain no real values.
- The attachment store lives outside the repository working tree and MUST be
  covered by `.gitignore` regardless, so encrypted blobs are never committed.
- Local TLS certificates are generated per developer and MUST NOT be committed.

**Additional constraints**:

- Production deployment targets are **out of scope**. No Dockerfiles, CI
  pipelines, cloud configuration, or production hardening are written until this
  constitution is amended to add a deployment target. The TLS and encryption
  requirements above apply in local development anyway, so that no security
  behavior is first exercised in production.
- The Go service MUST run and the Angular app MUST build with the commands
  documented in the feature `quickstart.md`.

## Development Workflow & Quality Gates

1. **Specify** — `/speckit-specify` produces `spec.md` on a feature branch.
   Requirements are written in terms of user-visible behavior, not
   implementation.
2. **Plan** — `/speckit-plan` produces `plan.md`. The Constitution Check gate
   MUST pass before design work begins and MUST be re-checked after design. Any
   violation is recorded in the plan's Complexity Tracking table with the simpler
   alternative that was rejected and why.
3. **Tasks** — `/speckit-tasks` produces `tasks.md`. A feature touching the API
   MUST include an OpenAPI task before its handler tasks. A feature touching the
   UI MUST include an RTL/Arabic task. A feature touching the schema MUST include
   a migration task. A feature accepting file uploads MUST include an encryption
   task and an attachment-access audit task. A feature that stores
   administrator-designated sensitive fields MUST include an encryption task and a
   task that excludes those fields from every search, sort, filter, and report. A
   feature writing business records MUST include an audit task.
4. **Implement** — code is written only after the above artifacts exist.
5. **Verify** — the manual procedure from `quickstart.md` is executed and its
   result recorded before the feature is closed. For any feature handling
   attachments, verification MUST include inspecting the stored file on disk and
   confirming it is unreadable ciphertext.

Work MUST happen on a feature branch. Direct commits to `main` for feature work
are prohibited.

## Governance

This constitution supersedes any other convention, habit, or preference in this
repository. Where a tool default, a generated file, or an agent instruction
conflicts with a principle here, this document wins.

**Amendment procedure**: An amendment is a written change to this file,
accompanied by a version bump and an updated Sync Impact Report. Any dependent
template affected by the amendment is updated in the same change. Amendments
apply going forward; already-shipped code is not retroactively rewritten unless
the amendment says so explicitly.

**Versioning policy** (semantic):

- **MAJOR** — a principle is removed or redefined incompatibly, or a fixed stack
  choice changes.
- **MINOR** — a principle or a section is added, or existing guidance is
  materially expanded.
- **PATCH** — clarification, wording, or typo fixes that do not change what is
  required.

**Compliance review**: Every plan runs the Constitution Check gate. Every review
verifies the change against Principles I, II, IV, VII, and VIII at minimum,
because those five fail silently — a missing role check, an unmirrored layout, a
float column, a plaintext file, and a missing audit row all look fine on screen.
Complexity that violates Principle VI MUST be justified in writing or removed.

**Runtime guidance**: Day-to-day technical context (commands, structure,
dependencies) lives in the active feature's `plan.md`, as directed by
`AGENTS.md` and `CLAUDE.md`. This constitution governs; the plan instructs.

**Version**: 1.2.0 | **Ratified**: 2026-08-29 | **Last Amended**: 2026-08-30
