# Feature Specification: Property Attachments with Extracted Text

**Feature Branch**: `003-property-attachments-ocr`

**Created**: 2026-08-30

**Status**: Draft

**Input**: User description: "adding the attachments to the properties and andding description to it make the attachment secure and encrypted aslo adding OCR to save the data from the attachments and search with it"

## Clarifications

### Session 2026-08-31

- Q: How much extracted text is kept per attachment? → A: 256 KB by default — roughly 100 to 150 pages of Arabic prose — and configurable rather than fixed in code, like the maximum upload size.
- Q: What happens when text extraction fails? → A: The system retries up to three times with a growing delay, then marks the attachment failed and stops. A user may additionally press "try again" on a failed or empty attachment, which restarts the cycle.
- Q: Is an attachment viewed inside the application or downloaded? → A: PDFs and images are viewed inline, decrypted and streamed per request. Nothing derived is ever cached — no thumbnails, no rendered copies on disk. Office documents download.

### Session 2026-08-30

- Q: Which file types may be attached, and how large may a file be? → A: pdf, png, jpg, jpeg, doc, docx, xls, xlsx, ppt, pptx, gif, bmp, tif, tiff. Maximum size defaults to 50 MB per file and is configurable rather than fixed in code.
- Q: Extracted text is a searchable plaintext copy of an encrypted document. How is it protected? → A: The uploader marks an attachment sensitive. A sensitive attachment's extracted text is encrypted at rest and excluded from document search, exactly as a sensitive custom field is in feature 002. Sensitive documents stay findable by their description, never by their contents.
- Q: Does extraction run locally, or may documents be sent to an external service? → A: Local only. A decrypted document never leaves this machine. Arabic recognition will be measurably worse as a result, which is why extracted text is correctable.
- Q: May a person review and correct extracted text? → A: Yes. The text is shown, editable, and marked as corrected once a person has changed it, with who changed it recorded. Re-extraction never silently overwrites a correction.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Attaching a document to a property (Priority: P1)

A staff member opens a property, attaches a scanned document — a title deed, a registry
extract, a photograph — and writes a short Arabic description saying what it is. The
attachment appears in a list on the property, showing its description, who added it, when,
and how large it is.

**Why this priority**: The register today records what a property *is* but holds none of
the paper that proves it. This is the whole point of the feature, and on its own it
already replaces a filing cabinet.

**Independent Test**: Open a property, attach a file with a description, and confirm it
appears in that property's attachment list with the right description, uploader, date, and
size — and that the stored file on disk is unreadable.

**Acceptance Scenarios**:

1. **Given** a property is open, **When** the user attaches a file and writes a
   description, **Then** the attachment is stored and listed against that property.
2. **Given** the upload form, **When** the user tries to save without a description,
   **Then** it is refused with an Arabic message, because an attachment nobody can
   identify is worse than none.
3. **Given** a file larger than the configured maximum, **When** the user tries to attach
   it, **Then** it is refused before any of it is stored, with an Arabic message stating
   the limit that is actually configured.
4. **Given** a file of a type the system does not accept, **When** the user tries to
   attach it, **Then** it is refused with an Arabic message listing what is accepted.
4a. **Given** a file renamed to look like an accepted type — an executable called
   `deed.pdf` — **When** the user tries to attach it, **Then** it is refused, because the
   type is judged from the content and not from the name.
5. **Given** an attachment was stored, **When** the file is inspected on disk, **Then** it
   is unreadable ciphertext and its filename reveals nothing about its contents.
6. **Given** the upload form, **When** the user marks the attachment sensitive, **Then**
   it is stored with its extracted text encrypted and withheld from document search, while
   the attachment itself stays readable and findable by its description.
7. **Given** an attachment was stored, **When** an administrator opens the records screen,
   **Then** an audit entry names who attached it, to which property, and when.
8. **Given** an upload that fails part way, **When** the request ends, **Then** no partial
   file and no orphaned record remain.

---

### User Story 2 - Reading an attachment back (Priority: P1)

A staff member opens a property, sees its attachments, and opens one to read it. The
system decrypts it for that request only. Every such access is recorded.

**Why this priority**: An attachment that cannot be read back is not stored, it is lost.
This and User Story 1 are one round trip and neither is useful alone.

**Independent Test**: Attach a file, open it from the property, confirm the content is
byte-identical to what was uploaded, and confirm an audit entry records the access.

**Acceptance Scenarios**:

1. **Given** an attachment exists, **When** an authorised user opens it, **Then** the
   original file is returned unchanged.
1a. **Given** a `pdf` or image attachment, **When** an authorised user views it, **Then**
   it is shown inside the application without a download step.
1b. **Given** an attachment has been viewed inline any number of times, **When** the
   attachment store is inspected, **Then** it holds exactly one encrypted file for that
   attachment and no thumbnail, preview, or rendered copy of any kind.
1c. **Given** an Office document, **When** the user opens it, **Then** it is downloaded
   rather than rendered.
2. **Given** an attachment exists, **When** it is opened, **Then** an audit entry records
   who opened it, which attachment, which property, and when.
3. **Given** a signed-out visitor, **When** they request an attachment directly by its
   address, **Then** they are refused and no content is returned.
4. **Given** an attachment whose stored bytes have been altered on disk, **When** anyone
   opens it, **Then** the request fails with an Arabic error and **no content at all** is
   returned — not a partial file, not a corrupted one.
5. **Given** any attachment, **When** its storage location is requested directly rather
   than through the application, **Then** nothing is served.

---

### User Story 3 - Correcting and removing attachments (Priority: P2)

A staff member fixes a description they typed wrongly, or removes an attachment added in
error.

**Why this priority**: Descriptions are typed by hand and will be wrong. Removal matters
less often but a wrong document attached to a property is a real problem.

**Independent Test**: Change an attachment's description and confirm the list shows the new
one; remove an attachment and confirm it leaves the property's list and the action is
recorded.

**Acceptance Scenarios**:

1. **Given** an attachment, **When** the user edits its description and saves, **Then**
   the new description is shown and the change is recorded.
2. **Given** an attachment, **When** the user removes it, **Then** a confirmation naming
   the attachment and its description is shown first.
3. **Given** the confirmation, **When** the user confirms, **Then** the attachment stops
   appearing on the property and the removal is recorded with who did it.
4. **Given** an archived property, **When** a user tries to attach, edit, or remove an
   attachment, **Then** it is refused until the property is restored.

---

### User Story 4 - Text is read out of the attachment (Priority: P2)

When a document is attached, the system reads whatever printed Arabic and Western text it
can find in it and keeps that text with the attachment, so the document's contents become
findable later rather than being a picture nobody can search.

**Why this priority**: This is what separates this feature from a folder of files. It
depends entirely on User Story 1 and is worthless without it.

**Independent Test**: Attach a scanned document with known legible text, wait for
extraction to finish, and confirm the extracted text contains the expected words and is
attributed to the right attachment.

**Acceptance Scenarios**:

1. **Given** a document is attached, **When** extraction completes, **Then** the text found
   in it is stored against that attachment and the attachment shows that it has been
   processed.
2. **Given** extraction is still running, **When** the user views the attachment, **Then**
   its state says so, rather than appearing to have no text.
3. **Given** a document with no readable text — a blank scan, a photograph of a building —
   **When** extraction completes, **Then** the attachment is marked as processed with no
   text found, which is a normal outcome and not an error.
4. **Given** extraction fails, **When** the user views the attachment, **Then** the failure
   is visible and the attachment itself is still readable and downloadable. **Extraction
   failing MUST never make the document itself inaccessible.**
4a. **Given** an extraction that fails transiently, **When** the system retries, **Then** it
   tries up to three times with a growing delay and succeeds without anyone intervening.
4b. **Given** an attachment marked failed, **When** the user presses "try again", **Then**
   extraction is attempted again and the attachment returns to the pending state.
4c. **Given** an attachment whose text a person has corrected, **When** the user asks for
   re-extraction, **Then** it is refused with an Arabic message saying the correction would
   be lost, and the correction survives.
4d. **Given** a user presses "try again" repeatedly, **When** an extraction is already
   running for that attachment, **Then** no second extraction starts.
5. **Given** a file type text cannot be read from, **When** it is attached, **Then** it is
   accepted and simply marked as not eligible for extraction.
6. **Given** an attachment is removed, **When** the removal completes, **Then** its
   extracted text is removed with it.

---

### User Story 5 - Finding a property by what its documents say (Priority: P3)

A staff member remembers a registry number, or a name written on a deed, but not which
property it belongs to. They search for it and find the property whose attachment contains
that text.

**Why this priority**: The reason the previous story exists. It depends on it.

**Independent Test**: Attach a document containing a distinctive phrase, wait for
extraction, search for that phrase, and confirm the property is returned and the matching
attachment is identified.

**Acceptance Scenarios**:

1. **Given** attachments with extracted text, **When** the user searches for a phrase that
   appears in one, **Then** the property holding it is returned, and the result says which
   attachment matched.
2. **Given** a search phrase written with different Arabic letter variants, diacritics, or
   tatweel, **When** the search runs, **Then** it matches the same way property names do.
3. **Given** a search matching nothing, **When** the results are empty, **Then** an Arabic
   message says so.
4. **Given** attachments the user's role may not read, **When** they search, **Then** no
   text from those attachments appears in their results.
4a. **Given** a sensitive attachment containing a distinctive phrase, **When** anyone
   searches that phrase — administrator included — **Then** the attachment is not returned,
   and the screen states in Arabic that sensitive documents are not searched.
5. **Given** an archived property, **When** a document search runs, **Then** it is excluded
   unless archived properties are explicitly included.

---

### User Story 6 - Correcting what was read out (Priority: P3)

Extraction from a scan is never perfect, especially in Arabic. A staff member opens an
attachment, sees the text that was read out of it, and corrects it so that searches find
it.

**Why this priority**: Without it, a bad extraction is permanently unsearchable and nobody
can tell why. With it, the failure is visible and fixable.

**Independent Test**: Attach a document, alter its extracted text, save, then search for a
word that only exists in the corrected version and confirm the property is found.

**Acceptance Scenarios**:

1. **Given** an attachment with extracted text, **When** the user views it, **Then** the
   text is shown as stored.
2. **Given** the text is shown, **When** the user corrects and saves it, **Then** searches
   use the corrected text from then on.
3. **Given** text was corrected by hand, **When** the attachment is viewed later, **Then**
   it is marked as corrected, and by whom, so nobody mistakes it for raw output.
3a. **Given** a sensitive attachment, **When** an authorised user views its extracted text,
   **Then** it is decrypted for that request only, and appears in no log or audit record.
4. **Given** text was corrected, **When** anything triggers re-extraction, **Then** the
   correction is not silently overwritten.

---

### Edge Cases

- What happens when a spreadsheet or presentation is attached? Its text is read directly
  out of the file rather than recognised from a picture of it, so the result is exact.
- What happens when a PDF mixes pages that carry text with pages that are scans? Each page
  is handled by its own family — read directly where there is a text layer, recognised
  where there is not.
- What happens when the maximum size is lowered after large files are already stored? Those
  attachments stay readable. The limit governs new uploads only.
- What happens when the same file is attached twice to one property? Both are kept, because
  two copies of a document may legitimately differ; the descriptions distinguish them.
- What happens when a very large document is attached? Extraction is bounded — beyond a
  stated page or size limit the attachment is stored and marked as too large to process,
  rather than blocking the upload.
- What happens when many attachments are uploaded at once? Each is processed independently;
  one failing extraction never affects another attachment or its property.
- What happens when a file fails extraction every time — a corrupt scan? It is retried three
  times, marked failed, and left alone. It stays readable and downloadable, and a person may
  still ask for another attempt, which will fail again and stop again.
- What happens when the system restarts while an extraction is pending? The attachment is
  still marked pending and MUST be picked up again rather than sitting pending forever.
- What happens when the master key is unavailable? No attachment can be read, uploads are
  refused, and the failure is explicit rather than silently storing readable files.
- What happens when a property is archived while an extraction is still running? The
  extraction completes and its text is stored; the property's archived state governs
  visibility, not the processing.
- What happens when extracted text is enormous — a hundred-page scan? Stored text is capped
  at the configured size, and the attachment records that it was truncated. A phrase that
  appears only past the cut is not findable, which is exactly why the truncation is shown
  on the attachment rather than hidden.
- What happens when the cap is lowered after long texts are stored? Existing text is left
  as it is. The new cap applies to later extractions only.
- What happens when the browser caches an inline-viewed document? The response is marked so
  that it is not written to the browser's disk cache, because a decrypted document sitting
  in a cache folder is the same leak as one sitting in the attachment store.
- What happens when a user without permission guesses an attachment's address? Refused by
  the server, whatever the interface shows.
- What happens when an ordinary attachment is promoted to sensitive after its text is
  already searchable? Its text is encrypted and leaves the search from that moment. Text
  already returned in someone's earlier results cannot be recalled, which is why the
  promotion is recorded with who did it and when.
- What happens when a sensitive attachment's text is corrected? The correction is
  re-encrypted; at no point is the corrected text stored readable.
- What happens to attachments when a property is archived? They stay with it, readable, and
  return intact when it is restored.

## Requirements *(mandatory)*

### Functional Requirements

#### Attaching and describing

- **FR-001**: Users MUST be able to attach one or more files to a property.
- **FR-002**: Every attachment MUST carry a description of 2 to 200 characters, required at
  upload and editable afterwards.
- **FR-003**: The system MUST record, for every attachment, its original filename, its
  size, its type, who attached it, and when.
- **FR-004**: The system MUST accept exactly these file types and refuse everything else
  with an Arabic message listing what is accepted:
  `pdf`, `png`, `jpg`, `jpeg`, `gif`, `bmp`, `tif`, `tiff`, `doc`, `docx`, `xls`, `xlsx`,
  `ppt`, `pptx`.
- **FR-004a**: The system MUST determine a file's type from its actual content, not from
  its filename extension, and MUST refuse a file whose content does not match an accepted
  type even when its name says otherwise.
- **FR-005**: The system MUST enforce a maximum file size, **defaulting to 50 MB**, and MUST
  refuse a larger file with an Arabic message stating the limit, before storing any part of
  it.
- **FR-005a**: The maximum file size MUST be configurable without changing code, and the
  configured value MUST be the one enforced and the one named in the refusal message.
- **FR-006**: The system MUST refuse attaching to, editing, or removing attachments on an
  archived property until it is restored.
- **FR-007**: Users MUST be able to edit an attachment's description.
- **FR-008**: Users MUST be able to remove an attachment, after a confirmation naming it.
- **FR-009**: Removing an attachment MUST remove its stored file and its extracted text.

#### Confidentiality

These follow from Principle VII of the constitution and are restated here because they are
the user's explicit requirement, not because this feature may choose otherwise.

- **FR-010**: Every attachment MUST be encrypted at rest with AES-256-GCM using envelope
  encryption: a fresh data key per file, stored only in wrapped form.
- **FR-011**: Plaintext bytes MUST NOT touch disk at any point, including as a temporary
  file that is deleted afterwards.
- **FR-012**: Stored files MUST be named by opaque identifier; the original filename is
  metadata only and MUST NOT determine the storage path.
- **FR-013**: Attachments MUST be served only through a handler that checks the caller's
  role and decrypts per request. No directly reachable storage location, no static file
  server, no shareable pre-signed path.
- **FR-013a**: Users MUST be able to view a `pdf` or an image attachment inside the
  application without downloading it. The content MUST be decrypted and streamed for that
  request only.
- **FR-013b**: The system MUST NOT write, cache, or retain any derived copy of an
  attachment — no thumbnail, no preview image, no rendered page, no extracted attachment on
  disk. Every view re-reads and re-decrypts the stored file. This follows Principle VII: a
  cached rendering is a second, unprotected copy of the document.
- **FR-013c**: Office documents — `doc`, `docx`, `xls`, `xlsx`, `ppt`, `pptx` — MUST be
  offered as a download rather than rendered in the application.
- **FR-013d**: Viewing an attachment inline MUST be recorded as an attachment access,
  exactly as downloading it is. Viewing is reading.
- **FR-014**: The authentication tag MUST be verified on every read; a failure MUST return
  an error and no content whatsoever.
- **FR-015**: The master key MUST come from the environment and MUST NOT appear in the
  database, a log, an error message, or the repository.
- **FR-016**: The system MUST refuse to accept uploads when the master key is unavailable,
  rather than storing anything unencrypted.

#### Reading text out of attachments

- **FR-017**: The system MUST attempt to extract text from every attachment of an eligible
  type, in Arabic and in Western characters. Eligibility falls into three families, and the
  system MUST use the right one for each type rather than treating every file as an image:
  - **Recognition from pixels** — `png`, `jpg`, `jpeg`, `gif`, `bmp`, `tif`, `tiff`, and
    any `pdf` page that carries no text layer. Text is read out of the image.
  - **Direct reading** — `docx`, `xlsx`, `pptx`, and any `pdf` page that already carries a
    text layer. The text is present in the file and MUST be read directly rather than
    recognised from a rendering, because reading it is both exact and cheaper.
  - **Legacy office formats** — `doc`, `xls`, `ppt`. These MUST be accepted and stored. If
    text cannot be read from one, the attachment MUST be marked as not eligible rather than
    failed, because nothing is wrong with the document.
- **FR-018**: Extraction MUST NOT block the upload; the user MUST be told the attachment is
  stored while extraction is still running.
- **FR-019**: Every attachment MUST carry a visible extraction state: pending, done, done
  with no text found, not eligible, too large to process, or failed.
- **FR-020**: A failed or empty extraction MUST NOT affect the attachment itself, which
  MUST remain readable and downloadable.
- **FR-020a**: The system MUST retry a failed extraction up to three times, waiting longer
  before each attempt, and MUST then mark the attachment failed and stop retrying by
  itself. Automatic retrying MUST be bounded so that one unreadable file cannot generate
  work forever.
- **FR-020b**: Users MUST be able to ask for extraction to be attempted again on an
  attachment whose state is failed or done-with-no-text-found, which restarts the bounded
  cycle.
- **FR-020c**: The system MUST refuse a manual re-extraction on an attachment whose text a
  person has corrected, with an Arabic message explaining that the correction would be
  lost. The correction MUST be cleared deliberately first. This is how FR-024b is honoured
  when re-extraction is requested rather than triggered.
- **FR-020d**: The system MUST NOT run more than one extraction at a time for the same
  attachment, however many times re-extraction is requested.
- **FR-020e**: The system MUST write an audit record when a person asks for re-extraction,
  naming the actor and the attachment.
- **FR-021**: The system MUST cap the amount of stored extracted text per attachment,
  **defaulting to 256 KB**, and MUST mark an attachment whose text was truncated so that a
  search missing a phrase from a very long document is explicable rather than mysterious.
- **FR-021a**: The extracted-text cap MUST be configurable without changing code, and the
  configured value MUST be the one enforced.
- **FR-021b**: Truncation MUST cut at a whole-character boundary, never mid-character, so
  that stored Arabic text is always valid text.
- **FR-021c**: Lowering the cap MUST NOT retroactively truncate text already stored; the cap
  governs extractions performed after the change.
- **FR-022**: Users MUST be able to mark an attachment **sensitive** when attaching it.
- **FR-022a**: A sensitive attachment's extracted text MUST be encrypted at rest under the
  same envelope scheme as the file itself, and MUST NOT be stored in readable form.
- **FR-022b**: A sensitive attachment's extracted text MUST be excluded from document
  search entirely, and the search screen MUST say so in Arabic rather than silently
  omitting those documents.
- **FR-022c**: The system MUST allow an attachment to be promoted from ordinary to
  sensitive at any time, which removes its text from search, and MUST refuse the reverse,
  because demoting would publish text that was accepted as confidential.
- **FR-022d**: A sensitive attachment MUST remain fully readable, downloadable, and
  findable by its description; sensitivity governs its **contents**, never its existence.
- **FR-023**: Text extraction MUST run entirely on the machine running the system. A
  document, decrypted or otherwise, MUST NOT be transmitted to any external service.
- **FR-024**: Users MUST be able to view an attachment's extracted text and correct it.
- **FR-024a**: The system MUST record that text was corrected by hand and by whom, so that
  corrected text is never mistaken for machine output.
- **FR-024b**: Re-extraction MUST NOT overwrite text a person has corrected.
- **FR-024c**: Viewing or correcting a **sensitive** attachment's extracted text MUST
  decrypt it only inside a handler that has already checked the caller's role, and the
  decrypted text MUST NOT reach a log, an error message, or an audit record.

#### Searching by document contents

- **FR-025**: Users MUST be able to search for text found in **non-sensitive** attachments
  and receive the properties whose attachments contain it.
- **FR-026**: Results MUST identify which attachment matched, not merely which property.
- **FR-027**: Document search MUST match after the same Arabic normalisation used for
  property names — insensitive to case, letter variants, diacritics, and tatweel.
- **FR-028**: Document search MUST exclude archived properties unless archived properties
  are explicitly included.
- **FR-029**: Document search MUST NOT return text from any attachment the caller is not
  permitted to read, and MUST NOT return text from any sensitive attachment regardless of
  the caller's role.

#### Access and recording

- **FR-030**: The system MUST require a valid signed-in session for every attachment
  operation.
- **FR-031**: The system MUST permit both administrators and managers to attach, read,
  describe, and remove attachments, because attachments are operational data.
- **FR-032**: The system MUST write an audit record for every attachment added, described,
  removed, and **every time one is opened or decrypted**, naming the actor, their role, the
  action, the attachment, its property, the UTC time, and the source address.
- **FR-033**: Audit records MUST NOT contain file contents or extracted text — identifiers
  and descriptions only.
- **FR-034**: The audit write MUST occur in the same transaction as the change it records.

#### Language and layout

- **FR-035**: Every user-visible string added by this feature MUST be Arabic, including
  file-type and size refusals, extraction states, and confirmations.
- **FR-036**: Every screen added by this feature MUST lay out right-to-left using logical
  layout rules.
- **FR-037**: File sizes, page counts, and dates MUST render in Western digits, and dates in
  Africa/Cairo local time.

### Key Entities

- **Attachment** — a file held against a property. Attributes: the property it belongs to, a
  required Arabic description, the original filename, size, and type, an opaque storage
  identifier, the wrapped data key and nonce needed to decrypt it, whether it is sensitive,
  who attached it and when, and its extraction state. Sensitivity moves in one direction
  only: ordinary may become sensitive, never the reverse.
- **Extracted Text** — the text read out of one attachment. Attributes: the attachment it
  came from, the text itself — stored readable for an ordinary attachment and as ciphertext
  with its wrapped key and nonce for a sensitive one — whether it was truncated, whether a
  person has corrected it and who, and when it was produced.
- **Audit Record** — the existing append-only record, extended in use to cover attachment
  additions, description changes, removals, and **reads**.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A staff member can attach a document with a description in under 60 seconds
  from opening the property.
- **SC-002**: 0 attachments are readable on disk — inspecting any stored file yields
  ciphertext, in 100% of attempts.
- **SC-003**: 0 plaintext bytes are written to disk at any point during an upload, including
  temporary files.
- **SC-004**: 100% of attachment reads appear in the audit records with the correct actor,
  attachment, and property.
- **SC-005**: 100% of attachments remain downloadable regardless of whether extraction
  succeeded, found nothing, or failed.
- **SC-005a**: A transient extraction failure resolves without human intervention in the
  majority of cases, and 0 attachments are retried automatically more than three times.
- **SC-005b**: 0 hand-corrected texts are lost to a re-extraction, whether automatic or
  requested.
- **SC-006**: An attachment is stored and acknowledged to the user within 5 seconds of
  upload for a file up to the configured maximum, independent of how long extraction takes.
- **SC-006a**: 0 files of an unaccepted type are stored, including files renamed to carry an
  accepted extension.
- **SC-007**: A staff member can find a property by a distinctive phrase in one of its
  documents in under 15 seconds.
- **SC-008**: 0 documents are served by any route that has not performed a role check.
- **SC-008a**: 0 derived copies of any attachment exist on disk at any time — after viewing
  every attachment inline, the store holds exactly one encrypted file per attachment and
  nothing else.
- **SC-008b**: 100% of inline views appear in the audit records, indistinguishable in
  completeness from downloads.
- **SC-009**: A tampered stored file yields 0 bytes of content and an explicit error, in
  100% of attempts.
- **SC-010**: 100% of screens and messages added by this feature are Arabic, and 0 physical
  left/right layout rules are used in their styling.
- **SC-011**: 0 sensitive attachments' extracted text is readable in the database, in a log,
  in an error body, or in any audit record — inspecting the stored row yields ciphertext.
- **SC-012**: 0 sensitive attachments are returned by document search, for any role, in 100%
  of attempts, and the search screen states the exclusion in Arabic.
- **SC-013**: 0 documents leave the machine — no outbound request carries attachment content
  during upload, extraction, or search.
- **SC-014**: A staff member can correct a mis-read phrase and find the property by the
  corrected wording in under 2 minutes.
- **SC-015**: 100% of attachments whose text was cut short are visibly marked as truncated.
- **SC-016**: 0 stored extracted texts contain a partial character at the truncation point.

## Assumptions

- Attachments belong to properties only. Attachments on tenants, leases, and payments are
  out of scope and expected with those features.
- Both roles may attach and read, because attachments are operational data, following
  Principle I. No per-attachment permission scheme is introduced.
- Extraction is best-effort. Arabic text recognition on scanned documents is imperfect, and
  the feature is designed so that an imperfect result degrades search rather than blocking
  anything.
- Extraction runs after the upload responds. A user never waits for it.
- The envelope encryption built for feature 002's sensitive custom fields is reused here
  rather than a second scheme being introduced, for both the file and a sensitive
  attachment's extracted text.
- Marking an attachment sensitive is done by whoever attaches it, either role, because only
  the person holding the document knows what is in it. This differs from feature 002, where
  designation is an administrator's act at field-definition time.
- Local-only extraction means Arabic recognition quality will be materially worse than a
  cloud service would give. The correction path in User Story 6 is the deliberate
  compensation for that, not an afterthought.
- Attachments follow the property. Archiving a property hides its attachments from the
  working views but destroys nothing, consistent with feature 002.
- Inline viewing costs a decryption per view rather than a cached rendering. That is the
  deliberate trade: Principle VII treats a cached thumbnail as a second copy of the
  document, and one encrypted copy that is read repeatedly is safer than two copies where
  the second is unprotected.
- Versioning of attachments is out of scope: replacing a document means removing one and
  attaching another, each recorded.
- Virus and malware scanning of uploads is out of scope for a local-only deployment, and
  is noted here so the omission is deliberate rather than overlooked. Office documents can
  carry macros; the system stores them and reads text out of them but never opens or
  executes them, so the exposure is to the text-reading component rather than to the
  machine.
- The accepted-type list spans three extraction families with genuinely different mechanics.
  The legacy binary formats — `doc`, `xls`, `ppt` — are the weakest case: they are accepted
  and stored like anything else, but text extraction from them may simply not be available,
  which the specification treats as "not eligible" rather than as a failure.
- Two limits are configuration rather than constants, at the user's request: the maximum
  upload size and the extracted-text cap. Both ship with a working default — 50 MB and
  256 KB — so the feature is usable before anyone configures anything, and both are read
  from configuration rather than compiled in. No other value in this feature is
  configurable; Principle VI rules out configurability nobody asked for.
