# Tasks: Properties Register

**Feature**: `002-properties-crud` | **Date**: 2026-08-30

**Input**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/openapi.yaml](./contracts/openapi.yaml),
[quickstart.md](./quickstart.md)

**Tests**: Constitution V sets no automated test gate. Two narrow Go unit tests are
included anyway, for the envelope crypto and the code generator, because that logic is
invisible on screen and getting it wrong is silent. Everything else is verified by the
manual procedure in `quickstart.md`.

**Constitution task obligations** (v1.2.0, Development Workflow step 3), each satisfied:
OpenAPI before handlers → T004 precedes every handler task. Migration → T005–T010.
RTL/Arabic → T088, T089. Encryption → T012. Search exclusion → T081. Audit → T014.

---

## Phase 1 — Setup

- [x] T001 Add `PMS_KEK` to `.env.example` with a comment that it is base64 of 32 random bytes and carries no value in the repository
- [x] T002 Read and validate `PMS_KEK` at startup in `api/internal/config/config.go`, refusing to start when it is absent or not 32 bytes once decoded
- [x] T003 Add `env-check` coverage for `PMS_KEK` and a `seed-demo` target to `Makefile`, keeping every path quoted
- [x] T004 Place the contract at `contracts/openapi.yaml` in the repository root contracts directory, copied from `specs/002-properties-crud/contracts/openapi.yaml`, and extend `make generate` in `Makefile` to generate both the Go interface and the Angular client from it

**Checkpoint**: `make env-check` passes; `make generate` produces code from the new contract.

---

## Phase 2 — Foundational (blocks every user story)

### Migrations

- [x] T005 Create `api/migrations/0009_audit_entity_columns.sql` adding the `audit_entity` enum and the nullable `entity_type` and `entity_id` columns plus their index, per data-model.md
- [x] T006 Create `api/migrations/0010_audit_actions_property.sql` adding the thirteen new `audit_action` values, **carrying `-- +goose NO TRANSACTION`** because PostgreSQL will not let a new enum value be used in the transaction that added it
- [x] T007 [P] Create `api/migrations/0011_lookup_tables.sql` with `property_type` and `area`, their normalised unique indexes and length checks, and the seeded values FR-026 requires
- [x] T008 Create `api/migrations/0012_property.sql` with `property_code_seq`, the `property` table, the archived-pair check, and every index from data-model.md — including the unique index on `code_normalized` that deliberately spans archived rows
- [x] T009 Create `api/migrations/0013_custom_fields.sql` with `custom_field_type`, `custom_field`, `custom_field_choice`, `property_field_value`, and `property_field_multi_value`, including the sensitive-text-only check and both value-shape checks
- [x] T010 Create `api/migrations/0014_property_grants.sql` granting `pms_app` on the seven new tables and the sequence, deliberately leaving `audit_log` at `SELECT, INSERT`
- [x] T011 Run `make migrate` and confirm all six apply cleanly and roll back cleanly, then re-apply

### Cross-cutting Go

- [x] T012 Create `api/internal/crypto/envelope.go` implementing AES-256-GCM seal and open with a per-value data key wrapped by the KEK, returning ciphertext, nonce, wrapped key, and wrap nonce, and verifying the tag on every open
- [x] T013 [P] Create `api/internal/crypto/envelope_test.go` covering the seal/open round trip, a tampered tag, a tampered ciphertext, and a wrong KEK — each of which must fail closed and return nothing
- [x] T014 Extend `api/internal/audit/writer.go` to carry `entity_type` and `entity_id`, and to require a transaction so a business write and its audit row cannot be issued separately
- [x] T015 [P] Export the Arabic normaliser from `api/internal/identity/canonical.go` for reuse on property names, codes, and lookup labels without duplicating the rules
- [x] T016 Create `api/internal/properties/store.go` with the connection plumbing and transaction helper every later store method uses
- [x] T017 Add the Arabic strings this feature needs to `api/internal/httpx/messages_ar.go`, including the duplicate-code, in-use, version-conflict, archived, and sensitive-not-searchable messages
- [x] T018 Extend `api/internal/httpx/errors.go` with the `in_use`, `version_conflict`, and `archived` error codes from the contract

### Cross-cutting Angular

- [x] T019 Regenerate the Angular client into `web/src/app/api/` from the contract and confirm no request or response type is hand-written
- [x] T020 [P] Add the `/properties` route tree and the admin-only settings child routes to `web/src/app/app.routes.ts`, reusing `requireSignedInGuard`, `requirePasswordChangedGuard`, and `requireAdminGuard`
- [x] T021 [P] Add the العقارات entry to the sidebar in `web/src/app/app.ts`

**Checkpoint**: schema applied, crypto tested, client generated, routes reachable. No user story may begin before this point.

---

## Phase 3 — User Story 1: Browsing and finding a property (P1)

**Goal**: A signed-in user sees every property with code, name, type, and area; searches by name or code; filters by type and area; pages; and sees the right Arabic message when there is nothing to show.

**Independent test**: Sign in, open العقارات, confirm seeded properties appear sorted by name; search a fragment and confirm narrowing; apply a type filter and confirm it combines with the search; page forward and back.

- [x] T022 [US1] Implement `ListProperties` in `api/internal/properties/store.go` with normalised name-and-code matching, type and area filters, the archived exclusion, name ordering, and a total count
- [x] T023 [US1] Implement the `listProperties` handler in `api/internal/httpx/property_handlers.go`, reading the role from the session and never from the request
- [x] T024 [US1] Wire `GET /properties` into `api/internal/httpx/server.go` behind the `authed()` wrapper so an unlisted route stays denied by default
- [x] T025 [P] [US1] Implement `ListPropertyTypes` and `ListAreas` in `api/internal/properties/lookup_store.go`, ordered by label
- [x] T026 [US1] Implement the `listPropertyTypes` and `listAreas` handlers in `api/internal/httpx/lookup_handlers.go` and wire both routes, readable by manager because the filter bar needs them
- [x] T027 [US1] Build the list screen in `web/src/app/features/properties/properties-list.component.ts` showing code, name, type, and area per row, in Arabic and RTL
- [x] T028 [P] [US1] Add the search box and the type and area filter controls to `web/src/app/features/properties/properties-list.component.ts`, combining rather than replacing one another
- [x] T029 [P] [US1] Add the paging control with page sizes 10, 25, 50, and 100 and a range indicator in Western digits to `web/src/app/features/properties/properties-list.component.ts`
- [x] T030 [US1] Add the two distinct Arabic empty states — register empty, and nothing matches the current search — to `web/src/app/features/properties/properties-list.component.ts`
- [ ] T031 [US1] Preserve search text, filters, page, and page size across navigation away and back, in `web/src/app/features/properties/properties-list.component.ts`
- [x] T032 [US1] Add `seed-demo` to `api/cmd/admintool` inserting the six design properties, so the list has something to show without a migration polluting the empty-state path

**Checkpoint**: US1 is independently demonstrable. Quickstart sections A and C pass.

---

## Phase 4 — User Story 2: Adding a property (P2)

**Goal**: A staff member adds a property; the server generates its reference code; duplicate names are accepted and duplicate codes are not.

**Independent test**: Add a property with a new name, confirm it appears with the next code; add a second with the same name and confirm it is accepted; confirm an audit record names who added it.

- [x] T033 [US2] Implement the reference-code generator in `api/internal/properties/code.go` using `property_code_seq`, zero-padded to three digits, with the bounded retry that research.md D-001 requires when an administrator override has already taken the next value
- [x] T034 [P] [US2] Create `api/internal/properties/code_test.go` covering generation, a collision with an administrator-set code, and exhaustion of the retry budget
- [x] T035 [US2] Implement `CreateProperty` in `api/internal/properties/store.go`, writing the property and its audit row in one transaction and rejecting a manager who supplied a code
- [x] T036 [US2] Implement the `createProperty` handler in `api/internal/httpx/property_handlers.go`, validating name length, type, and area, and returning field-level Arabic messages
- [x] T037 [US2] Wire `POST /properties` in `api/internal/httpx/server.go`
- [x] T038 [US2] Build the add-property form in `web/src/app/features/properties/property-form.component.ts` with name, type, and area, in Arabic and RTL
- [x] T039 [P] [US2] Convert Eastern Arabic digits to Western on entry in `web/src/app/features/properties/property-form.component.ts`, reusing the rule feature 001 established
- [x] T040 [US2] Surface the duplicate-code and validation errors from the API as Arabic field messages in `web/src/app/features/properties/property-form.component.ts`

**Checkpoint**: US2 works on top of US1. Quickstart section B passes.

---

## Phase 5 — User Story 3: Seeing everything about one property (P2)

**Goal**: Clicking a row opens that property's own screen showing every stored field and its creation and modification record.

**Independent test**: Click any row, confirm code, name, type, area, and the created/modified record appear in Africa/Cairo time; go back and confirm the list state survived.

- [x] T041 [US3] Implement `GetProperty` in `api/internal/properties/store.go` returning the property with its creator and modifier usernames, and reading an archived property successfully
- [x] T042 [US3] Implement the `getProperty` handler in `api/internal/httpx/property_handlers.go`, returning the Arabic not-found message for an unknown identifier
- [x] T043 [US3] Wire `GET /properties/{propertyId}` in `api/internal/httpx/server.go`, registering it after the literal `/properties/search` route per the routing note in research.md D-006
- [x] T044 [US3] Build the detail screen in `web/src/app/features/properties/property-detail.component.ts` showing every stored field and the record section
- [x] T045 [P] [US3] Render timestamps in Africa/Cairo, Gregorian, Western digits, in `web/src/app/features/properties/property-detail.component.ts`
- [x] T046 [US3] Make list rows in `web/src/app/features/properties/properties-list.component.ts` open the detail screen, and confirm the deep link works when pasted directly

**Checkpoint**: US3 works. Quickstart section D steps 1–2 and 7–8 pass.

---

## Phase 6 — User Story 4: Correcting a property (P3)

**Goal**: A staff member edits name, type, and area; a stale save is refused rather than silently overwriting.

**Independent test**: Edit a property's area and confirm the list reflects it; open the same property in two tabs, save both, and confirm the second is refused with the current values shown.

- [x] T047 [US4] Implement `UpdateProperty` in `api/internal/properties/store.go` comparing and incrementing `version`, refusing an archived property, and writing the audit row in the same transaction
- [x] T048 [US4] Implement the `updateProperty` handler in `api/internal/httpx/property_handlers.go` returning `version_conflict` and `archived` distinctly so the Arabic message can differ
- [x] T049 [US4] Wire `PUT /properties/{propertyId}` in `api/internal/httpx/server.go`
- [x] T050 [US4] Add edit mode to `web/src/app/features/properties/property-form.component.ts`, carrying the version read with the property
- [x] T051 [US4] Present the stale-save refusal in the UI with the current values, never a silent overwrite, in `web/src/app/features/properties/property-form.component.ts`
- [x] T052 [P] [US4] Implement `ChangePropertyCode` in `api/internal/properties/store.go` and its `PATCH /properties/{propertyId}/code` handler, admin-only, auditing both the old and the new code
- [x] T053 [US4] Show the reference code read-only to a manager and editable to an administrator in `web/src/app/features/properties/property-detail.component.ts`

**Checkpoint**: US4 works. Quickstart section D passes in full, including the concurrency step.

---

## Phase 7 — User Story 5: Archiving a property (P3)

**Goal**: Archiving removes a property from the working list without destroying it; it can be listed, opened, and restored, and its code stays reserved forever.

**Independent test**: Archive a property, confirm it leaves the list and appears among archived; try to reuse its code and confirm refusal; restore it and confirm every field returns.

- [x] T054 [US5] Implement `ArchiveProperty` and `RestoreProperty` in `api/internal/properties/store.go`, setting and clearing the archived pair, incrementing version, and auditing each as a distinct action
- [x] T055 [US5] Implement the `archiveProperty` and `restoreProperty` handlers in `api/internal/httpx/property_handlers.go`
- [x] T056 [US5] Wire `POST /properties/{propertyId}/archive` and `POST /properties/{propertyId}/restore` in `api/internal/httpx/server.go`
- [x] T057 [US5] Confirm no route, handler, or store method anywhere in `api/internal/properties/` can delete a property row, per FR-022
- [x] T058 [US5] Add the archive confirmation naming the property to `web/src/app/features/properties/property-detail.component.ts`
- [x] T059 [P] [US5] Add the archived-properties view to `web/src/app/features/properties/properties-list.component.ts` via the `includeArchived` filter, showing when each was archived and by whom
- [x] T060 [US5] Add the restore action and its confirmation to `web/src/app/features/properties/property-detail.component.ts`
- [x] T061 [US5] Confirm in `api/internal/properties/store.go` that an archived code is refused on create and on code change, with the Arabic message from `api/internal/httpx/messages_ar.go` that names it as belonging to an archived property

**Checkpoint**: US5 works. Quickstart section E passes.

---

## Phase 8 — User Story 6: Maintaining the type and area lists (P3)

**Goal**: An administrator adds, renames, and removes property types and areas; renames propagate; in-use entries cannot be removed; a manager is refused by the server.

**Independent test**: Add an area, use it, rename it and confirm the property shows the new name, try to remove it and confirm refusal with a count; then confirm a manager is refused all three by direct API call.

- [x] T062 [US6] Implement create, rename, and delete for both lookups in `api/internal/properties/lookup_store.go`, auditing each with the entity type that distinguishes a type from an area
- [x] T063 [US6] Implement the in-use count in `api/internal/properties/lookup_store.go`, counting archived properties too, so the Arabic message can state how many
- [x] T064 [US6] Implement the six lookup write handlers in `api/internal/httpx/lookup_handlers.go`, each with a flat administrator check
- [x] T065 [US6] Wire the six lookup write routes in `api/internal/httpx/server.go`, all admin-only
- [x] T066 [US6] Confirm the `ON DELETE RESTRICT` constraint refuses the delete even when the count races, not only the count
- [x] T067 [US6] Build the reusable lookup panel in `web/src/app/features/settings/lookup-panel.component.ts`, written once because the two lists have identical shape and rules
- [x] T068 [P] [US6] Instantiate the panel for property types at `web/src/app/features/settings/property-types/property-types.component.ts`
- [x] T069 [P] [US6] Instantiate the panel for areas at `web/src/app/features/settings/areas/areas.component.ts`
- [x] T070 [US6] Add both screens to the Settings navigation in `web/src/app/features/settings/settings.component.ts`, visible to administrators only
- [x] T071 [US6] Surface the duplicate-label and in-use refusals as Arabic messages in `web/src/app/features/settings/lookup-panel.component.ts`

**Checkpoint**: US6 works. Quickstart section H steps 1–5 pass, and section G's first three curl checks return 403.

---

## Phase 9 — User Story 7: Defining custom property fields (P3)

**Goal**: An administrator defines fields of four types; they appear on the property form and detail screen; sensitive free-text fields are encrypted at rest; definitions in use cannot be destructively changed.

**Independent test**: Define one field of each type, fill all four on a property, confirm the detail screen shows them; mark a text field sensitive, save a value, and confirm the database holds ciphertext only.

- [x] T072 [US7] Implement custom field definition create, rename, choice add, choice remove, and delete in `api/internal/properties/custom_field_store.go`, auditing each
- [x] T073 [US7] Enforce in `api/internal/properties/custom_field_store.go` that only a free-text field may be sensitive, that a choice type arrives with at least one choice, and that type, sensitivity, and choice removal are refused once any value exists
- [x] T074 [US7] Implement `GetCustomValues` and `SaveCustomValues` in `api/internal/properties/custom_value_store.go`, sealing a sensitive value with `api/internal/crypto/envelope.go` before it reaches the database and never writing it to `text_value`
- [x] T075 [US7] Confirm in `api/internal/properties/custom_value_store.go` that a decrypted value never reaches a log, an error body, or an audit row — audit rows name the field, never the value
- [x] T076 [US7] Implement the four custom-field handlers in `api/internal/httpx/custom_field_handlers.go`, all administrator-only, and wire them in `api/internal/httpx/server.go`
- [x] T077 [US7] Extend the create and update property handlers in `api/internal/httpx/property_handlers.go` to accept and persist custom values in the same transaction as the property and its audit row
- [x] T078 [US7] Extend the detail response in `api/internal/httpx/property_handlers.go` to decrypt sensitive values only after the session role check has already passed
- [x] T079 [US7] Build the field-definition screen at `web/src/app/features/settings/custom-fields/custom-fields.component.ts` with the four types and the choice editor
- [x] T080 [P] [US7] Add the Arabic note to `web/src/app/features/settings/custom-fields/custom-fields.component.ts` stating that national ID, passport, bank account, and IBAN values belong in a sensitive field
- [x] T081 [US7] Render one input per field type — text, dropdown, multi-select, checkbox — in `web/src/app/features/properties/property-form.component.ts`, every field optional
- [x] T082 [US7] Render every defined field and its value, empty ones included, in `web/src/app/features/properties/property-detail.component.ts`
- [x] T083 [US7] Surface the in-use refusal with its count, and the sensitivity-fixed refusal, as Arabic messages in `web/src/app/features/settings/custom-fields/custom-fields.component.ts`

**Checkpoint**: US7 works. Quickstart section F steps 1–5 and 9–14 pass, and section H steps 6–10 pass.

---

## Phase 10 — User Story 8: Searching on custom fields (P3)

**Goal**: An advanced search filters on built-in fields and every non-sensitive custom field, combining filters with AND, and refusing a sensitive field at the server.

**Independent test**: With fields defined and varied values stored, filter on one custom field and confirm exactly the matching properties return; add a second filter and confirm narrowing; call the API by hand with a sensitive field and confirm 400.

- [x] T084 [US8] Implement `SearchProperties` in `api/internal/properties/search_store.go`, resolving every `fieldId` against the definitions before building the query and binding every value as a parameter, never interpolating
- [x] T085 [US8] **Refuse a filter naming a sensitive field with `400` in `api/internal/properties/search_store.go`**, so the exclusion is enforced by the server and not by the absence of a control in the UI — this is the search-exclusion task Constitution VII requires
- [x] T086 [US8] Check each operator against its field's type in `api/internal/properties/search_store.go` and refuse a mismatch
- [x] T087 [US8] Implement the `searchProperties` handler in `api/internal/httpx/property_handlers.go` and wire `POST /properties/search` in `api/internal/httpx/server.go` **before** the `/properties/{propertyId}` pattern
- [x] T088 [US8] Omit sensitive fields from the definitions the advanced search screen offers, in `api/internal/httpx/custom_field_handlers.go`
- [x] T089 [US8] Build the advanced search screen at `web/src/app/features/properties/advanced-search.component.ts` with one filter per field type
- [x] T090 [P] [US8] State in Arabic in `web/src/app/features/properties/advanced-search.component.ts` that sensitive fields cannot be searched
- [x] T091 [P] [US8] Add the clear-all-filters action to `web/src/app/features/properties/advanced-search.component.ts`
- [x] T092 [US8] Exclude archived properties from the advanced search unless explicitly included, in `api/internal/properties/search_store.go`

**Checkpoint**: US8 works. Quickstart section F steps 6–8 and section H steps 11–13 pass.

---

## Phase 11 — Polish & Cross-Cutting

- [x] T093 Audit every new rule in `web/src/styles.scss` and the feature stylesheets for physical CSS properties and convert any to logical, using the same grep gate feature 001 uses — this is the RTL task Constitution II requires
- [x] T094 [P] Confirm every user-visible string added under `web/src/app/features/properties/` and `web/src/app/features/settings/` is Arabic, with no English placeholder reachable on any screen
- [x] T095 [P] Confirm every directional icon added under `web/src/app/features/properties/` mirrors under RTL, and that non-directional ones do not
- [x] T096 Run `go build ./...` and `go vet ./...` in `api/` and confirm both are clean
- [x] T097 [P] Run `make run-web` and confirm the bundle builds with no error and no unused import left by these changes
- [x] T098 Confirm every write path in `api/internal/properties/` performs its audit insert inside the same transaction as its change, and that a forced audit failure rolls the change back
- [x] T099 Grep the API logs after exercising the feature and confirm no KEK, no wrapped key, and no decrypted sensitive value appears, checking every log call added under `api/internal/properties/` and `api/internal/crypto/`
- [ ] T100 Measure the first page of the list at 500 properties, seeded via `api/cmd/admintool`, and confirm it renders within 2 seconds, per SC-003
- [ ] T101 Measure an advanced search combining a built-in filter and two custom-field filters at 500 properties against `POST /properties/search` and confirm it answers within 2 seconds, per SC-008c
- [ ] T102 **Execute `quickstart.md` sections A through H and record every result in its table.** Constitution V makes this binding: the feature is not done until every row reads PASS

---

## Dependencies

```text
Phase 1 Setup
      │
Phase 2 Foundational  ── blocks everything below
      │
      ├─ Phase 3  US1 Browse (P1)  ── the MVP
      │        │
      │        ├─ Phase 4  US2 Add (P2)
      │        │        │
      │        │        ├─ Phase 6  US4 Modify (P3)
      │        │        └─ Phase 9  US7 Custom fields (P3) ── extends the US2 form
      │        │
      │        ├─ Phase 5  US3 Detail (P2)
      │        │        └─ Phase 7  US5 Archive (P3)
      │        │
      │        └─ Phase 8  US6 Lookup management (P3) ── independent of US2-US5
      │
      └─ Phase 10 US8 Advanced search (P3) ── requires US7
                    │
              Phase 11 Polish
```

**Story dependencies stated plainly**:

- **US1** depends only on the foundation. It is the MVP.
- **US2** and **US3** depend on US1 for the list to add to and click from.
- **US4** and **US5** extend US2 and US3 respectively.
- **US6** is independent of US2–US5 — it can be built any time after the foundation, and it is what makes section G of the quickstart demonstrable.
- **US7** extends the property form built in US2 and the detail screen built in US3.
- **US8** requires US7, because there are no custom fields to search until they can be defined.

---

## Parallel opportunities

Within Phase 2: T007 runs alongside T005–T006; T013, T015, T020, and T021 are independent of one another.

Within a story phase, the marked `[P]` tasks touch different files and have no ordering between them:

- US1 — T025 alongside T022–T024; T028 and T029 alongside each other
- US2 — T034 alongside T035–T037; T039 independent of T040
- US6 — T068 and T069 are the same panel instantiated twice and can be done together
- US7 — T080 independent of the handler work
- US8 — T090 and T091 independent of the store work
- Polish — T094, T095, and T097 are independent checks

The two Go test files, T013 and T034, can be written at any point after their subject exists.

---

## Implementation strategy

**MVP is Phase 1 + Phase 2 + Phase 3 (US1)** — 32 tasks. That delivers a working Arabic
properties register with search, filters, and paging over seeded data, which is already
the screen the request asked to see.

**Then, in order of what each adds**:

1. US2 and US3 (Phases 4–5) complete the original request: add a property, click it, read it.
2. US4 and US5 (Phases 6–7) finish the four operations, and US5 is where the never-destroyed guarantee becomes real.
3. US6 (Phase 8) makes the register extensible without a developer, and unlocks quickstart section G.
4. US7 and US8 (Phases 9–10) are the custom-field system, the largest single increment, and the only part that touches encryption.
5. Phase 11 is not optional. T102 in particular is what Constitution V means by done.

**Stopping points that leave a coherent product**: after Phase 3, after Phase 7, and after
Phase 11. Stopping mid-Phase 9 leaves definitions that the property form does not yet
render, which is the one place where a partial delivery is visibly broken.
