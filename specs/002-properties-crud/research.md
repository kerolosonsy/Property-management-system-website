# Phase 0 Research: Properties Register

**Feature**: `002-properties-crud` | **Date**: 2026-08-30

Eleven decisions. Each states what was chosen, why, and what was rejected. Where a
decision is forced by the constitution rather than by preference, the principle is named.

---

## D-001 — Reference code generation and the override collision

**Decision**: A PostgreSQL sequence `property_code_seq` supplies the numeric part; the
generated form is `P-` followed by the sequence value zero-padded to at least three
digits, in Western digits. The code lives in its own column with a unique index over a
normalised form. Generation is attempt-and-retry: take the next sequence value, try to
insert, and on a unique violation take the next value again, up to five attempts before
failing the request.

**Rationale**: A sequence is the only mechanism that survives concurrent inserts without a
lock, and it never moves backwards, which is exactly FR-017b's "never reuse". The retry
loop exists because FR-017c lets an administrator type `P-005` by hand while the sequence
still sits at 3 — two legitimate features that collide on the third insert afterwards.
Five attempts is enough that the loop can only be exhausted by a deliberate adversary, and
a failed request is a better answer than a duplicate code.

**Alternatives considered**:

- *`max(code) + 1` at insert time* — rejected. It requires a table lock to be correct
  under concurrency, and archiving a property would silently free its number for reuse,
  breaking FR-017b.
- *A UUID as the reference code* — rejected. Codes are read aloud, written on paper, and
  typed by staff; a UUID is unusable for the purpose the code exists to serve.
- *Refusing administrator overrides that fall inside the generated range* — rejected. It
  is an arbitrary rule to explain to a user, and the retry loop handles the collision
  without needing it.

---

## D-002 — Custom field values: relational rows, not a JSONB column

**Decision**: Three tables. `custom_field` holds definitions; `custom_field_choice` holds
the ordered choices of a dropdown or multi-select; `property_field_value` holds one row
per property per field for the scalar types, and `property_field_multi_value` holds one
row per selected choice for multi-selects.

**Rationale**: Constitution IV: "Every relationship MUST be a real foreign key. Orphaned
units, leases, or payments MUST be impossible at the schema level." FR-027f (rename
propagates because a value refers to its field by identity) and FR-027g (a field or a
choice cannot be removed while values reference it) are both foreign-key questions. With
real references, `ON DELETE RESTRICT` enforces FR-027g in the database and the application
merely reports it.

**Alternatives considered**:

- *A `custom_values jsonb` column on `property`* — rejected. It is genuinely simpler to
  write and would have been the Principle VI answer if Principle IV did not exist. It
  cannot carry a foreign key, so "is this choice still in use" becomes an application-code
  scan that is wrong the moment two requests race. Recorded in the plan's Complexity
  Tracking.
- *One wide table with a column per field type and a discriminator* — rejected for
  multi-select, which is inherently one-to-many and would need an array column, taking
  the foreign key away again.

---

## D-003 — Envelope encryption for designated sensitive values

**Decision**: AES-256-GCM from `crypto/aes` and `crypto/cipher` in the Go standard
library. Per **value**, a fresh random 256-bit data key and a fresh 96-bit nonce; the
ciphertext, the nonce, the wrapped data key, and the wrap nonce are four columns on
`property_field_value`. The KEK is 32 bytes, read from `PMS_KEK` as base64, and wraps the
data key with AES-256-GCM as well. The authentication tag is verified on every open; a
failure is a hard error and returns nothing.

**Rationale**: This is the scheme Principle VII already mandates for attachments, applied
to a value instead of a file, so the project has one encryption story rather than two. A
per-value data key means compromising one wrapped key exposes one cell. The standard
library covers all of it, so Principle VI's "no dependency this feature does not need"
holds.

**Alternatives considered**:

- *One data key per field, wrapped once, with a per-value nonce* — rejected. It is cheaper
  by one wrap operation per write, and at this scale that saving is invisible; the cost is
  that one recovered data key exposes every property's value for that field.
- *`pgcrypto` and encryption inside the database* — rejected. The KEK would have to be
  passed to the database or stored in it, and Principle VII forbids the key living where
  the ciphertext lives.
- *Deriving the data key from the KEK and the value's identity* — rejected. It removes the
  wrapped-key column but makes key rotation require re-encrypting every value, which
  Principle VII explicitly says rotation must not require.

---

## D-004 — Widening the audit table

**Decision**: Migration 0009 adds two nullable columns to `audit_log`: `entity_type` (a new
enum: `property`, `property_type`, `area`, `custom_field`) and `entity_id` (text). The
existing `target_account_id` and `target_username_snapshot` columns are untouched.
Migration 0010 adds the new `audit_action` enum values in a `-- +goose NO TRANSACTION`
migration.

**Rationale**: Principle VIII requires "entity type, entity identifier" on every record.
Feature 001's table can only name an account. Nullable columns leave every existing row and
query valid, which matters because the records screen from feature 001 is already shipped.
`entity_id` is text rather than uuid because a property's business identity is its
reference code, and a record that says which code changed is more useful a year later than
one that says which uuid did.

The `NO TRANSACTION` split is not stylistic. PostgreSQL will not let a new enum value be
*used* in the same transaction that added it, and goose wraps each migration in one by
default, so adding values and inserting rows that use them must be separate migrations.
This is the kind of thing that passes review and fails at runtime, so it is written down.

**Alternatives considered**:

- *A separate `business_audit_log` table* — rejected. Two audit tables means two places to
  look and two sets of privileges to keep append-only; the question "who touched this"
  should have one answer.
- *Replacing the enum with a text column* — rejected. It would loosen feature 001's
  existing constraint for this feature's convenience, which Principle VI's surgical-change
  rule forbids.

---

## D-005 — Optimistic concurrency

**Decision**: An integer `version` column on `property`, starting at 1 and incremented by
every update. The property representation carries it; the update request carries the
version the client last read; the server refuses with `409 conflict` when they differ.

**Rationale**: FR-020 requires a stale save to be refused rather than silently overwrite,
and SC-009 puts a number on it: zero silent losses. An integer comparison is exact.

**Alternatives considered**:

- *Comparing `updated_at`* — rejected. Two updates inside the same clock tick compare
  equal, and the failure is a lost update, which is the one outcome SC-009 forbids.
- *HTTP `ETag` and `If-Match`* — rejected. It is the more correct HTTP answer, but the
  generated TypeScript client surfaces headers awkwardly and the version would end up
  hand-threaded on the Angular side, which Principle III's no-hand-written-types rule
  makes unattractive. A field in the body is generated for free on both sides.

---

## D-006 — Advanced search: server-side whitelisting

**Decision**: The advanced-search request carries filters as a list of
`{fieldId, operator, value}` objects. The server loads every custom field definition,
rejects any `fieldId` it does not know, rejects any field marked sensitive, checks the
operator against the field's type, and only then builds the query — with the field id as a
bound parameter, never as interpolated SQL. Sensitive fields are absent from the
advanced-search metadata the client fetches, so a well-behaved UI never offers them and a
badly-behaved one is refused anyway.

**Rationale**: Two requirements meet here. FR-027s4 excludes sensitive fields from search,
and Principle I says the server decides, never the client. Enforcing the exclusion only by
omitting the filter from the UI would make a hidden input the security boundary — which
Principle I names explicitly as carrying "zero security weight". Building SQL from
client-supplied field names would be an injection vector; resolving ids against the
definition table first removes the possibility rather than escaping it.

**Routing note**: `/properties/search` and `/properties/{propertyId}` share a prefix. They
do not collide by method — search is POST only and the id path has no POST — but Go's
stdlib `ServeMux` matches literal segments before wildcards only when the literal pattern
is registered, so `POST /api/v1/properties/search` MUST be registered explicitly. It is
worth knowing that a `GET /properties/search` would otherwise fall into the id route and
fail uuid parsing with a confusing 400 rather than a 404.

**Alternatives considered**:

- *A general query language in a string parameter* — rejected. Far more surface than eight
  filter shapes need, and every bit of it would have to be parsed and validated. Principle
  VI.
- *Filtering sensitive fields out in the Angular component* — rejected outright by
  Principle I.

---

## D-007 — Search over name and code

**Decision**: Two stored normalised columns, `name_normalized` and `code_normalized`,
written by the application using the same Arabic normaliser feature 001 uses for
usernames. `code_normalized` carries a unique index. Name search is a case-folded
substring match against `name_normalized`. No trigram index and no `pg_trgm` extension.

**Rationale**: Normalising once on write is cheaper and more predictable than normalising
on every read, and it makes the uniqueness of the code an actual database constraint
rather than an application check. Reusing feature 001's normaliser is what makes FR-007's
"insensitive to Arabic letter variants, diacritics, and tatweel" mean the same thing
across the product.

`pg_trgm` is omitted deliberately. At 500 rows a sequential scan answers well inside
SC-003's two seconds, and Principle VI forbids a dependency this feature does not need. If
the register ever reaches a size where this hurts, adding the extension is a migration and
no application change.

**Alternatives considered**:

- *An expression index over a normalising SQL function* — rejected. It would put the
  normalisation rules in two languages, and they would drift.
- *PostgreSQL full-text search* — rejected. Staff search for fragments of a name
  ("جرجس"), which is a substring problem, not a word-stemming problem.

---

## D-008 — Archive as a state, not a second table

**Decision**: Two nullable columns on `property`: `archived_at` and `archived_by`. Active
means `archived_at IS NULL`. The list, the search, and the advanced search all filter on
it by default; an explicit request includes archived rows. The unique index on
`code_normalized` covers every row regardless of state.

**Rationale**: FR-022 forbids destruction entirely, so an archived property is the same
row in a different state and every foreign key pointing at it stays valid — which is what
makes units and leases in later features safe. The unique index spanning both states is
what implements FR-022d and FR-017b: an archived code stays taken.

**Alternatives considered**:

- *Moving archived rows to a `property_archive` table* — rejected. Every future foreign
  key would need to point at one of two tables, and restoring would be a cross-table move
  that can half-fail.
- *A `status` enum with room for more states* — rejected. There are exactly two states and
  no requirement suggests a third; Principle VI forbids the speculative third.

---

## D-009 — Lookup rename propagation and the in-use check

**Decision**: `property.property_type_id` and `property.area_id` are foreign keys with
`ON DELETE RESTRICT`. Renaming is an update of the label on the lookup row and nothing
else. The in-use refusal (FR-025) is reported by counting referencing properties, archived
included, before attempting the delete; the `RESTRICT` constraint is the actual guarantee
and the count exists only to make the Arabic message specific.

**Rationale**: FR-024b's "refer by identity rather than by copied text" is what makes
rename free. Doing the count *and* keeping `RESTRICT` looks redundant but is not: the
count produces a helpful message, the constraint produces correctness under a race where
a property is created between the count and the delete.

**Alternatives considered**:

- *`ON DELETE SET NULL`* — rejected. It would leave properties with no type, which FR-016
  forbids.
- *Trusting the count alone and omitting `RESTRICT`* — rejected. Principle IV: the database
  enforces correctness, the application does not merely hope for it.

---

## D-010 — What is taken from the design source

**Decision**: From `Property Management System.html` this feature takes the list layout
and its filter chips, the paging control and its 10/25/50/100 page sizes, the lookup
management panel shape, the advanced-search form layout, and the Arabic wording of labels
and empty states. It does **not** take the file's markup, its combined
properties-and-units screen, or its unit-level concepts.

**Rationale**: Constitution II requires every plan consuming a design to say what it takes
and to audit the result. The design's own stylesheet mixes physical and logical CSS; the
vendored token file from feature 001 already converted what it uses, and any new rule this
feature writes is logical-only, verified by the same grep that gates feature 001.

**Alternatives considered**:

- *Porting the design's combined screen wholesale* — rejected. It renders units, which are
  out of scope, and would require inventing the unit entity to fill it.

---

## D-011 — Seeded data

**Decision** (corrected by migration 0015 — see data-model.md): Migration 0011 seeds the
property types (تجاري، سكني) and the areas from the design source, because FR-026 requires the register to be usable before an administrator
configures anything. Sample properties are **not** seeded by a migration; the existing
`admintool` gains a `seed-demo` command that inserts the six properties from the design
for manual verification.

**Rationale**: Configuration the feature requires belongs in a migration; demonstration
data does not, because a migration cannot be un-run selectively and the manual procedure
needs to start from a known, deliberately created state.

**Alternatives considered**:

- *Seeding sample properties in the migration too* — rejected. It would make the empty-state
  path of FR-010 unreachable without deleting rows by hand.
- *No seeding at all* — rejected. It contradicts FR-026 and makes the first run of
  quickstart section B impossible without a database session.
