# Tasks: Property Attachments with Extracted Text

**Feature**: `003-property-attachments-ocr` | **Date**: 2026-08-31

**Input**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/openapi.yaml](./contracts/openapi.yaml),
[quickstart.md](./quickstart.md)

**Tests**: Constitution V sets no automated test gate. Three Go unit test files are included
anyway, for the places where a mistake is silent and invisible on screen: the chunked
seal/open round trip with truncation and reorder rejection, the content sniffer against
files renamed to lie, and the extraction-family router.

**Constitution task obligations** (v1.3.0), each satisfied by a named task:
OpenAPI before handlers → T005 precedes every handler task. Migration → T006–T009.
RTL/Arabic → T091. Encryption (uploads) → T011. **Attachment-access audit** → T014, verified
by T037. **Search exclusion for derived text** → T072. Audit on business writes → T014.

---

## Phase 1 — Setup

- [ ] T001 Add `PMS_ATTACHMENT_STORE`, `PMS_ATTACHMENT_MAX_BYTES` (default 52428800), and `PMS_EXTRACT_TEXT_MAX_BYTES` (default 262144) to `.env.example` with comments, carrying no real values
- [ ] T002 Load and validate the three new settings in `api/internal/config/config.go`, refusing to start when the store path is missing, unwritable, or inside the repository working tree
- [ ] T003 Add the attachment store path to `.gitignore` and confirm `git check-ignore` reports it, so encrypted bodies can never be committed
- [ ] T004 Add `PMS_ATTACHMENT_STORE` and both limits to the `env-check` target in `Makefile`, reporting the store path and the two limits without printing any secret
- [ ] T005 Merge `specs/003-property-attachments-ocr/contracts/openapi.yaml` into the root `contracts/openapi.yaml`, preserving every feature 001 and 002 path, and regenerate with `make generate`

**Checkpoint**: `make env-check` passes; `make generate` produces the new client and server interface.

---

## Phase 2 — Foundational (blocks every user story)

### Migrations

- [ ] T006 Create `api/migrations/0016_attachment.sql` with the `attachment_extract_state` enum, the `attachment` table, all four CHECK constraints, and both indexes per data-model.md
- [ ] T007 Add the one-way sensitivity trigger and its function to `api/migrations/0016_attachment.sql`, so demotion is refused by the database and not only by the application
- [ ] T008 Create `api/migrations/0017_attachment_text.sql` with the `attachment_text` table, the one-shape CHECK, the corrected-pair CHECK, and the partial search index on `body_normalized IS NOT NULL`
- [ ] T009 Create `api/migrations/0018_attachment_grants.sql` granting `pms_app` on both new tables, deliberately leaving `audit_log` at `SELECT, INSERT`
- [ ] T010 Run `make migrate`, confirm 0016–0018 apply and roll back cleanly, then re-apply and verify the trigger refuses a direct `UPDATE ... SET is_sensitive=false`

### Encryption and storage

- [ ] T011 Create `api/internal/crypto/stream.go` implementing chunked AES-256-GCM over `io.Reader`/`io.Writer` — 64 KiB chunks, one data key per file wrapped by the KEK, per-chunk nonce from an 8-byte prefix plus a 4-byte counter, and **the chunk index plus a final-chunk flag bound into each chunk's additional data**
- [ ] T012 [P] Create `api/internal/crypto/stream_test.go` covering the round trip, a tampered chunk, **two chunks swapped**, a truncated tail, and a wrong KEK — every one of which must fail closed and yield no plaintext
- [ ] T013 Create `api/internal/blobstore/store.go` writing and reading files under the configured root, named by attachment id with no extension and sharded two levels, per research.md D-011
- [ ] T014 Extend `api/internal/audit/writer.go` usage for attachments: add the attachment actions, and provide a helper that writes a **read** record and commits before any content is streamed, per research.md D-010

### Extraction plumbing

- [ ] T015 Create `api/internal/extract/sniff.go` determining content type from leading bytes — stdlib detection plus explicit magic checks for OOXML, legacy OLE, PDF, and the image signatures — never from the filename
- [ ] T016 [P] Create `api/internal/extract/sniff_test.go` covering each accepted type, an executable renamed `.pdf`, and a ZIP that is not an OOXML document
- [ ] T017 Create `api/internal/extract/probe.go` detecting at startup whether `tesseract` (with the `ara` pack) and `pdftotext`/`pdftoppm` are present, recording capabilities for the router to consult
- [ ] T018 Create `api/internal/extract/router.go` choosing a family per file and, for PDF, per page, and returning `not_eligible` with a reason when a required tool is absent rather than failing
- [ ] T019 [P] Create `api/internal/extract/router_test.go` asserting the family chosen for each accepted type and that a missing tool yields `not_eligible`, never `failed`

### Cross-cutting

- [ ] T020 Create `api/internal/attachments/store.go` with the connection plumbing and transaction helper the later store methods use
- [ ] T021 Add this feature's Arabic strings to `api/internal/httpx/messages_ar.go`, including type and size refusals, the six extraction states, the demotion refusal, and the sensitive-not-searchable notice
- [ ] T022 [P] Extend `api/internal/httpx/errors.go` with `too_large`, `unsupported_type`, `integrity_failed`, and `key_unavailable`
- [ ] T023 [P] Regenerate the Angular client into `web/src/app/api/` and confirm no request or response type is hand-written
- [ ] T024 [P] Add this feature's Arabic strings to `web/src/app/shared/messages.ts`
- [ ] T025 [P] Add the attachment routes to `web/src/app/app.routes.ts`, reusing `requireSignedInGuard` and `requirePasswordChangedGuard`

**Checkpoint**: schema applied, encryption tested, sniffer tested, client generated. No user story may begin before this point.

---

## Phase 3 — User Story 1: Attaching a document (P1)

**Goal**: A staff member attaches a file with an Arabic description; it is encrypted on the way in and appears in the property's list.

**Independent test**: Attach a file with a description, confirm it appears with the right metadata, and confirm the stored file on disk is unreadable ciphertext with an opaque name.

- [ ] T026 [US1] Implement `CreateAttachment` in `api/internal/attachments/store.go`, writing the row and its audit record in one transaction
- [ ] T027 [US1] Implement the upload handler in `api/internal/httpx/attachment_handlers.go`, **streaming the multipart part straight through the sniffer and the chunked encryptor into the store without buffering the whole file and without any temporary file**
- [ ] T028 [US1] Enforce the configured size limit in `api/internal/httpx/attachment_handlers.go` by counting bytes as they stream, refusing with `413` before anything is committed to the store
- [ ] T029 [US1] Refuse an unaccepted type with `400` in `api/internal/httpx/attachment_handlers.go`, based on the sniffer's verdict and never on the filename
- [ ] T030 [US1] Refuse upload with `503` when the master key is unavailable, in `api/internal/httpx/attachment_handlers.go`, storing nothing
- [ ] T031 [US1] Refuse attaching to an archived property with `409` in `api/internal/httpx/attachment_handlers.go`
- [ ] T032 [US1] Clean up the store file and the row together when an upload fails part way, in `api/internal/attachments/store.go`, so no orphan of either kind remains
- [ ] T033 [US1] Wire `POST /properties/{propertyId}/attachments` and `GET /properties/{propertyId}/attachments` in `api/internal/httpx/server.go`
- [ ] T034 [US1] Build the attachment panel in `web/src/app/features/properties/attachments-panel.component.ts` with the upload form, the required description, the sensitivity checkbox, and the list
- [ ] T035 [P] [US1] Show each attachment's description, uploader, date in Africa/Cairo, and size in Western digits in `web/src/app/features/properties/attachments-panel.component.ts`
- [ ] T036 [US1] Add the panel to `web/src/app/features/properties/property-detail.component.ts` so attachments appear on the property

**Checkpoint**: quickstart section A passes.

---

## Phase 4 — User Story 2: Reading it back (P1)

**Goal**: An authorised user downloads or views a document; it decrypts per request, is recorded, and nothing is cached.

**Independent test**: Download an attachment and diff it against the original; view a PDF inline; confirm both produced audit records and that the store still holds exactly one file.

- [ ] T037 [US2] Implement the content handler in `api/internal/httpx/attachment_content_handler.go`, writing and **committing the read audit record before the first byte is streamed**, per research.md D-010
- [ ] T038 [US2] Stream decrypted chunks to the response in `api/internal/httpx/attachment_content_handler.go`, verifying each chunk's tag and aborting with no further content on failure
- [ ] T039 [US2] Return `422` with zero bytes when the stored file fails authentication, in `api/internal/httpx/attachment_content_handler.go`, never a partial or corrupted file
- [ ] T040 [US2] Set `Cache-Control: no-store` and `Pragma: no-cache` on every content response in `api/internal/httpx/attachment_content_handler.go`, so the browser writes no decrypted copy to disk
- [ ] T041 [US2] Honour `disposition=inline` only for `pdf` and image types in `api/internal/httpx/attachment_content_handler.go`, serving everything else as a download
- [ ] T042 [US2] Wire `GET /attachments/{attachmentId}/content` and `GET /attachments/{attachmentId}` in `api/internal/httpx/server.go`
- [ ] T043 [US2] Build the inline viewer in `web/src/app/features/properties/attachment-viewer.component.ts` for PDF and images, rendering from the streamed response and **caching nothing**
- [ ] T044 [P] [US2] Offer a download rather than a viewer for Office types in `web/src/app/features/properties/attachments-panel.component.ts`, driven by `canViewInline` from the contract
- [ ] T045 [US2] Confirm no route, static handler, or dev-server rule serves the attachment store directory, in `api/internal/httpx/server.go` and `web/angular.json`

**Checkpoint**: quickstart section B passes. US1 and US2 together are the round trip and the MVP.

---

## Phase 5 — User Story 3: Descriptions, sensitivity, removal (P2)

**Goal**: Descriptions can be fixed, attachments can be promoted to sensitive, and an attachment added in error can be removed.

**Independent test**: Edit a description; promote an attachment to sensitive and confirm demotion is refused by the API and by the database; remove an attachment and confirm the file, the text, and the row are all gone while the audit record remains.

- [ ] T046 [US3] Implement `UpdateAttachment` in `api/internal/attachments/store.go` for the description, auditing the change in the same transaction
- [ ] T047 [US3] Implement promotion to sensitive in `api/internal/attachments/store.go`: seal the existing text, write the cipher columns, and **null both `body` and `body_normalized` in the same statement**, so the row is never observable holding both
- [ ] T048 [US3] Refuse demotion with `409` and an Arabic message in `api/internal/httpx/attachment_handlers.go`, before the database trigger is reached
- [ ] T049 [US3] Implement `DeleteAttachment` in `api/internal/attachments/store.go`, removing the row, cascading the text, and deleting the store file, with the audit record written in the same transaction as the row
- [ ] T050 [US3] Refuse description edits, promotion, and removal on an archived property with `409` in `api/internal/httpx/attachment_handlers.go`
- [ ] T051 [US3] Wire `PATCH /attachments/{attachmentId}` and `DELETE /attachments/{attachmentId}` in `api/internal/httpx/server.go`
- [ ] T052 [US3] Add description editing and the removal confirmation naming the attachment to `web/src/app/features/properties/attachments-panel.component.ts`
- [ ] T053 [P] [US3] Add the promote-to-sensitive control with its one-way warning to `web/src/app/features/properties/attachments-panel.component.ts`

**Checkpoint**: quickstart section C passes.

---

## Phase 6 — User Story 4: Text comes out of the documents (P2)

**Goal**: Text is read out of each attachment by the right family, after the upload responds, with bounded retries and honest states.

**Independent test**: Attach one document of each family, confirm each reaches the right state with the right text, and confirm a failing extraction never makes its document unreadable.

- [ ] T054 [US4] Implement OOXML text reading in `api/internal/extract/ooxml.go` using `archive/zip` and `encoding/xml` only, with no third-party dependency
- [ ] T055 [US4] Implement PDF handling in `api/internal/extract/pdf.go`: detect a page's text layer with the Go PDF library, read it with `pdftotext`, and rasterise the rest with `pdftoppm` — **all subprocesses fed over pipes with `-` for input and output, never a file path**
- [ ] T056 [US4] Implement image recognition in `api/internal/extract/image.go` invoking `tesseract stdin stdout -l ara+eng` over pipes
- [ ] T057 [US4] Mark legacy `doc`, `xls`, and `ppt` as `not_eligible` in `api/internal/extract/router.go`, storing them normally
- [ ] T058 [US4] Implement the worker in `api/internal/attachments/worker.go`, claiming a due row with a conditional `UPDATE` so two workers cannot take the same attachment
- [ ] T059 [US4] Implement bounded retry in `api/internal/attachments/worker.go` — three attempts at 30 s, 2 min, 10 min, then `failed` and no longer due
- [ ] T060 [US4] Cap stored text at the configured size in `api/internal/attachments/worker.go`, **cutting on a whole-character boundary** and setting `truncated`
- [ ] T061 [US4] Seal the text for a sensitive attachment before storing it, in `api/internal/attachments/worker.go`, never writing it readable even briefly
- [ ] T062 [US4] Write only a reason code to `extract_error` in `api/internal/attachments/worker.go`, never any part of the document's content
- [ ] T063 [US4] Start the worker alongside the listener in `api/cmd/server/main.go` and stop it cleanly on shutdown
- [ ] T064 [US4] Requeue rows left `pending` by a restart, in `api/internal/attachments/worker.go`, so no attachment is stranded
- [ ] T065 [P] [US4] Show the extraction state per attachment in Arabic in `web/src/app/features/properties/attachments-panel.component.ts`, distinguishing empty and not-eligible from failed

**Checkpoint**: quickstart sections D and E pass.

---

## Phase 7 — User Story 5: Finding a property by its documents (P3)

**Goal**: Searching document text returns the properties holding a match, and never touches a sensitive attachment.

**Independent test**: Search a phrase from an ordinary document and get the property; search a phrase that exists only in a sensitive document, as an administrator, and get nothing.

- [ ] T066 [US5] Store the normalised form alongside ordinary extracted text in `api/internal/attachments/worker.go`, using the same Arabic normaliser as property names
- [ ] T067 [US5] Implement `SearchDocuments` in `api/internal/attachments/search_store.go` matching the normalised column, joining to the property, and returning which attachment matched
- [ ] T068 [US5] Exclude archived properties from document search unless explicitly included, in `api/internal/attachments/search_store.go`
- [ ] T069 [US5] Implement the search handler in `api/internal/httpx/attachment_handlers.go` and wire `POST /attachments/search` in `api/internal/httpx/server.go`
- [ ] T070 [US5] Return `sensitiveExcluded: true` from the search response in `api/internal/httpx/attachment_handlers.go` so the client can state the exclusion
- [ ] T071 [US5] Add document search to `web/src/app/features/properties/advanced-search.component.ts`, showing the matching attachment beside each property
- [ ] T072 [US5] **Verify sensitive attachments are unsearchable structurally** — confirm in `api/internal/attachments/worker.go` and `search_store.go` that a sensitive row carries no `body_normalized` at all, so no query, correct or careless, can return it. This is the search-exclusion task Constitution VII requires
- [ ] T073 [P] [US5] State in Arabic on the search screen that sensitive documents are not searched, in `web/src/app/features/properties/advanced-search.component.ts`

**Checkpoint**: quickstart section F steps 5–7 pass.

---

## Phase 8 — User Story 6: Correcting what was read (P3)

**Goal**: A person can read the extracted text, fix it, and have searches use the correction.

**Independent test**: Correct a mis-read phrase, search the corrected wording, find the property, and confirm a re-extraction request is refused rather than discarding the correction.

- [ ] T074 [US6] Implement `GetAttachmentText` in `api/internal/attachments/store.go`, decrypting a sensitive attachment's text only after the caller's role has been checked
- [ ] T075 [US6] Implement `CorrectAttachmentText` in `api/internal/attachments/store.go`, recording who corrected it and when, and re-sealing when the attachment is sensitive
- [ ] T076 [US6] Update the normalised form when ordinary text is corrected, in `api/internal/attachments/store.go`, so searches use the correction
- [ ] T077 [US6] Implement re-extraction in `api/internal/attachments/store.go`, permitted from `failed` and `empty`, returning the attachment to `pending`
- [ ] T078 [US6] **Refuse re-extraction with `409` when the text has been corrected**, in `api/internal/attachments/store.go`, so no automatic or requested extraction can discard a correction
- [ ] T079 [US6] Refuse a second concurrent extraction for the same attachment in `api/internal/attachments/store.go`
- [ ] T080 [US6] Audit a re-extraction request in `api/internal/httpx/attachment_handlers.go`, naming the actor and the attachment
- [ ] T081 [US6] Wire `GET`/`PUT /attachments/{attachmentId}/text` and `POST /attachments/{attachmentId}/text/reextract` in `api/internal/httpx/server.go`
- [ ] T082 [US6] Build the text screen in `web/src/app/features/properties/attachment-text.component.ts` showing the text, the truncation flag, and who corrected it
- [ ] T083 [P] [US6] Add the correction form and the "try again" control to `web/src/app/features/properties/attachment-text.component.ts`, surfacing the correction-would-be-lost refusal in Arabic
- [ ] T084 [US6] Confirm no decrypted text reaches a log, an error body, or an audit row, across `api/internal/attachments/` and `api/internal/httpx/`

**Checkpoint**: quickstart sections E and H pass.

---

## Phase 9 — Polish & Cross-Cutting

- [ ] T085 **Confirm no plaintext byte of any attachment is ever written to disk** — grep `api/internal/extract/` and `api/internal/attachments/` for `os.CreateTemp`, `ioutil.TempFile`, and any file path passed to a subprocess, and confirm every extraction command line uses `-` for input and output
- [ ] T086 Confirm no derived copy is produced anywhere — grep `api/internal/` for thumbnail, preview, and rendering writes, and confirm the store holds exactly one file per attachment after repeated inline viewing
- [ ] T087 Confirm no attachment or derived text leaves the machine — grep `api/internal/extract/` and `api/internal/attachments/` for outbound HTTP clients, and confirm the only subprocesses invoked are the local tools
- [ ] T088 Grep the API logs after exercising the feature and confirm no KEK, wrapped key, decrypted text, or file content appears, checking every log call added under `api/internal/attachments/`, `api/internal/extract/`, and `api/internal/blobstore/`
- [ ] T089 Confirm every attachment write and every attachment read has an audit row, and that reads are recorded before content is served, across `api/internal/httpx/`
- [ ] T090 Run `go build ./...`, `go vet ./...`, and `go test ./...` in `api/` and confirm all three are clean
- [ ] T091 Audit every new rule in `web/src/styles.scss` and the attachment components for physical CSS properties and convert any to logical — the RTL task Constitution II requires
- [ ] T092 [P] Confirm every user-visible string added under `web/src/app/features/properties/` is Arabic, with no English placeholder reachable on any screen
- [ ] T093 [P] Run `make run-web` and confirm the bundle builds with no error and no unused import left by these changes
- [ ] T094 Measure a 50 MB upload against `POST /properties/{propertyId}/attachments` and confirm it is stored and acknowledged within 5 seconds, per SC-006, with resident memory staying bounded rather than tracking file size
- [ ] T095 Measure inline viewing of a large PDF against `GET /attachments/{attachmentId}/content?disposition=inline` and confirm the first bytes stream without the whole file being buffered
- [ ] T096 **Execute `quickstart.md` sections A through H and record every result in its table.** Constitution V makes this binding: the feature is not done until every row reads PASS

---

## Dependencies

```text
Phase 1 Setup
      │
Phase 2 Foundational  ── blocks everything below
      │
      ├─ Phase 3  US1 Attach (P1) ─┐
      │                             ├─ together these are the MVP: a document
      ├─ Phase 4  US2 Read (P1) ───┘   that goes in and comes back out
      │        │
      │        ├─ Phase 5  US3 Describe / promote / remove (P2)
      │        │
      │        └─ Phase 6  US4 Extraction (P2)
      │                 │
      │                 ├─ Phase 7  US5 Document search (P3)
      │                 └─ Phase 8  US6 Correction (P3)
      │
      └─ Phase 9 Polish
```

**Story dependencies stated plainly**:

- **US1 and US2 are one round trip.** Either alone is useless: storing a document you cannot
  read back is losing it, and there is nothing to read back without US1. They are the MVP
  together.
- **US3** extends US1's panel and needs nothing from extraction.
- **US4** depends on US1 only — it processes what was stored.
- **US5** depends on US4: there is nothing to search until text exists.
- **US6** depends on US4 for the same reason, and interacts with US5 because a correction
  changes what search finds.

---

## Parallel opportunities

Within Phase 2: T012, T016, T019, T022, T023, T024, and T025 touch different files and have
no ordering between them; the three test files can be written any time after their subject
exists.

Within a story phase, the `[P]` tasks are independent:

- US1 — T035 alongside T034's structure
- US2 — T044 independent of the viewer
- US3 — T053 independent of T052
- US4 — T065 independent of the worker
- US5 — T073 independent of the store work
- Polish — T092 and T093 are independent checks

The three extraction families, T054–T056, are separate files and can be written in parallel
once the router in T018 exists.

---

## Implementation strategy

**MVP is Phase 1 + Phase 2 + Phase 3 + Phase 4** — 45 tasks. That delivers documents that go
in encrypted and come back out intact, viewable inline, with every read recorded. It is the
filing cabinet, and it is useful before a single word is extracted.

**Then, in order of what each adds**:

1. **US3** (Phase 5) makes the register correctable — descriptions get typed wrongly, and a
   document attached to the wrong property needs removing.
2. **US4** (Phase 6) is the largest single increment and the one with the outside
   dependencies. Everything before it works without `tesseract` or `poppler` installed.
3. **US5** (Phase 7) is what makes extraction worth having.
4. **US6** (Phase 8) is what makes extraction survivable, given that local Arabic recognition
   will be imperfect. Priced P3, but closer to load-bearing than that suggests.
5. **Phase 9 is not optional.** T085, T086, and T087 are the three claims this feature makes
   that cannot be seen on a screen, and T096 is what Constitution V means by done.

**Stopping points that leave a coherent product**: after Phase 4, after Phase 6, and after
Phase 9. Stopping mid-Phase 6 leaves attachments stuck `pending` with no worker to advance
them, which is the one place a partial delivery looks broken rather than merely incomplete.
