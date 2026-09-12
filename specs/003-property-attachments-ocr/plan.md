# Implementation Plan: Property Attachments with Extracted Text

**Branch**: `003-property-attachments-ocr` | **Date**: 2026-08-31 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/003-property-attachments-ocr/spec.md`

## Summary

Attach documents to a property with a required Arabic description; encrypt every one at
rest; read text out of them locally; make that text searchable unless the uploader marked
the attachment sensitive, in which case the text is encrypted and never searched.

The work divides into four parts that are genuinely independent of one another. **A file
store** that streams AES-256-GCM in fixed chunks, so a 50 MB upload never sits whole in
memory and never touches disk in the clear. **A text pipeline** with three families —
direct reading for OOXML and PDF text layers, recognition for images and scanned pages,
and honest refusal for legacy binary Office formats. **A job runner** carried on the
attachment row itself, so a restart resumes rather than stranding work. **A viewer** that
streams decrypted bytes per request and caches nothing, anywhere, including the browser.

Two new system prerequisites and two new Go dependencies, each recorded in Complexity
Tracking with what was rejected.

## Technical Context

**Language/Version**: Go 1.27 (`api/`), TypeScript 5.9 on Angular 22.1 (`web/`)

**Primary Dependencies**: existing — `jackc/pgx/v5`, `pressly/goose/v3`, `oapi-codegen`,
`typescript-angular` generator. **No new Go dependency.** The plan originally budgeted for
a PDF-parsing library to count pages and find text layers; `pdfinfo` from the poppler
package already answers both and ships alongside `pdftotext` and `pdftoppm`, so the
dependency was dropped during implementation. OOXML needs only `archive/zip` and
`encoding/xml`; chunked AES-256-GCM needs only `crypto/aes` and `crypto/cipher`.

**System prerequisites (new, and neither is currently satisfied on this machine)**:

| Tool | Purpose | State here |
|---|---|---|
| `tesseract` ≥ 5 with the **`ara`** language pack | recognition from pixels | binary present at 5.5.2, **`ara` pack missing — only `eng` is installed** |
| `poppler` (`pdftotext`, `pdftoppm`, `pdfinfo`) | PDF text layer, page count, and rasterising scanned pages | **absent entirely** |

Both are invoked as subprocesses over pipes, never handed a file path. Both are checked at
startup and their absence degrades extraction to "not eligible" rather than failing uploads.

**Storage**: PostgreSQL 17 for metadata, extracted text, and job state. Encrypted file
bodies live outside the repository working tree in a store directory named by opaque
identifier.

**Testing**: Manual, per Constitution V, driven by `quickstart.md`. Go unit tests are
written for the two places where a mistake is silent: the chunked seal/open round trip
including truncation and chunk-reorder rejection, and the content-type sniffer against
files renamed to lie about themselves.

**Target Platform**: Local development only. TLS on both listeners.

**Project Type**: Web application in a monorepo — `web/` and `api/`.

**Performance Goals**: A 50 MB upload is stored and acknowledged within 5 seconds
(SC-006). Inline viewing streams first bytes without buffering the whole file. Extraction
runs outside the request and never delays either.

**Constraints**: No plaintext byte of any attachment on disk, ever, including subprocess
input. No derived copy — no thumbnail, no rendered page, no cached preview. No attachment
or derived text leaves the machine. Arabic-only UI, RTL, logical CSS. Every read audited.

**Scale/Scope**: Hundreds of attachments, up to 50 MB each, up to 256 KB of text each.
Under ten concurrent users. 6 user stories, 58 requirements, 21 success criteria.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Derived from `.specify/memory/constitution.md` **v1.3.0**, which was amended on 2026-08-30
specifically to admit this feature's uploader-designated sensitive attachments.

**Initial evaluation — before Phase 0:**

- [x] **I. Authorization** — PASS. Every operation carries `x-required-role: manager`,
      which an administrator also satisfies; attachments are operational data (FR-031).
      There is no administrator-only operation in this feature, and no configuration
      endpoint at all. Role is read from the server-verified session.
- [x] **II. Arabic RTL** — PASS. All new strings Arabic; logical CSS only; the viewer and
      the attachment list are new screens audited by the same grep gate features 001 and
      002 use.
- [x] **III. Contract-First** — PASS. `contracts/openapi.yaml` is written in Phase 1 before
      any handler. Upload is `multipart/form-data` and download is a binary response; both
      are expressed in the contract, and the Angular client is generated from it.
- [x] **IV. Data Integrity** — PASS. Attachment → property and extracted text → attachment
      are real foreign keys. Three forward-only migrations. No money. UTC `timestamptz`.
- [x] **V. Manual Verification** — PASS with a stated deviation. `quickstart.md` carries
      the full procedure. **This feature has no configuration operations**, so the
      principle's manager-denied-configuration step does not apply literally. Section F
      substitutes the equivalent boundary this feature does have: nobody — administrator
      included — may demote a sensitive attachment or search its text, verified past the
      interface.
- [x] **VI. Simplicity** — PASS with four entries in Complexity Tracking. Two new system
      binaries and one new Go dependency are more than this project has taken on before,
      and each is recorded with the simpler option that was rejected.
- [x] **VII. Encryption** — PASS, and only under v1.3.0. Chunked AES-256-GCM with the chunk
      index bound into each chunk's additional data, so a reordered or truncated file fails
      to open rather than decrypting to something plausible. Plaintext never touches disk,
      including on the way to a subprocess. No derived copy is written anywhere. Sensitive
      attachments' text is encrypted and excluded from search for every role. Nothing is
      sent off the machine.
- [x] **VIII. Audit** — PASS. Every upload, description change, removal, re-extraction
      request, **download, and inline view** writes an append-only row in the same
      transaction, carrying entity type and identifier on the columns migration 0009 added.
      Reads are audited here, which ordinary record reads are not — Principle VIII requires
      exactly that for attachments.

**Re-evaluation after Phase 1 design**: see [Post-Design Constitution Re-Check](#post-design-constitution-re-check).

## Project Structure

### Documentation (this feature)

```text
specs/003-property-attachments-ocr/
├── plan.md              # This file
├── spec.md              # 6 stories, 58 requirements, 7 clarifications
├── research.md          # Phase 0 — twelve decisions with rejected alternatives
├── data-model.md        # Phase 1 — migrations 0016-0018, states, encryption layout
├── quickstart.md        # Phase 1 — prerequisites and the binding manual procedure
├── contracts/
│   └── openapi.yaml     # Phase 1 — the API contract
├── checklists/
│   └── requirements.md  # 16/16
└── tasks.md             # Phase 2 — produced by /speckit-tasks
```

### Source Code (repository root)

```text
api/
├── migrations/
│   ├── 0016_attachment.sql            # attachment table, states, indexes
│   ├── 0017_attachment_text.sql       # extracted text, plain and encrypted
│   └── 0018_attachment_grants.sql     # pms_app privileges
├── internal/
│   ├── crypto/
│   │   └── stream.go                  # NEW — chunked AES-256-GCM over io.Reader/Writer
│   ├── blobstore/                     # NEW — opaque-id file store outside the repo
│   ├── extract/                       # NEW — the three families, subprocess pipes
│   │   ├── sniff.go                   # content-type from bytes, not filename
│   │   ├── ooxml.go                   # archive/zip + encoding/xml, stdlib only
│   │   ├── pdf.go                     # text layer, then rasterise-and-recognise
│   │   └── image.go                   # tesseract over stdin/stdout
│   ├── attachments/                   # NEW — store, service, job runner
│   ├── httpx/                         # upload, download, view, text handlers
│   └── config/                        # extended: store path, size cap, text cap
└── cmd/server/                        # worker started alongside the listener

web/src/app/features/properties/
├── attachments-panel.component.ts     # NEW — list, upload, description, sensitivity
├── attachment-viewer.component.ts     # NEW — inline PDF and image, no caching
└── attachment-text.component.ts       # NEW — view, correct, re-extract
```

**Structure Decision**: Three new Go packages, each with one job — `blobstore` knows where
bytes live, `crypto/stream` knows how they are protected, `extract` knows how to read them.
They do not know about each other; `attachments` composes them. That separation is what
makes the "no plaintext on disk" rule checkable in one place rather than argued about
across a dozen handlers.

## Complexity Tracking

> Four entries. This feature takes on more outside machinery than 001 or 002, and Principle
> VI requires each addition to be justified in writing rather than absorbed quietly.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| `tesseract` + Arabic language pack as a system prerequisite | FR-017 requires reading Arabic text out of scanned images, and Q2 of the clarifications forbids sending documents to a service that could do it remotely. | There is no pure-Go Arabic OCR of usable quality. Writing one is not a feature, it is a research project. Shipping without recognition would reduce the feature to OOXML text reading and silently drop the scanned documents that are the whole point. |
| `poppler` (`pdftotext`, `pdftoppm`) as a system prerequisite | A PDF is two different problems: pages with a text layer must be read exactly, pages that are scans must be rasterised before recognition. Both need PDF machinery. | Rasterising a PDF in pure Go has no mature implementation. Treating every PDF as a scan would discard exact text and re-recognise it badly. Treating every PDF as text would return nothing for scans, which are the common case here. |
| ~~`github.com/ledongthuc/pdf`~~ — **withdrawn during implementation** | The plan expected to need a Go PDF parser to count pages and detect text layers. | Not needed. `pdfinfo` reports the page count and ships with the poppler package already required, and `pdftotext -f N -l N` answers the per-page text-layer question directly. The dependency was budgeted, then found unnecessary, so it was not taken. Recorded rather than deleted because the reasoning is worth keeping. |
| Chunked AES-256-GCM rather than the single-shot envelope feature 002 built | GCM is a single-message construction: sealing 50 MB requires the whole file in memory on the way in and again on the way out, and gives no way to stream a viewer's first bytes. | Reusing 002's `Seal`/`Open` unchanged would work and is tempting. It would mean holding two 50 MB buffers per concurrent upload, and would make inline viewing buffer the entire document before showing page one. The chunk index is bound into each chunk's additional data so the framing cannot be reordered or truncated undetectably. |

## Post-Design Constitution Re-Check

Re-run after `research.md`, `data-model.md`, `contracts/openapi.yaml`, and `quickstart.md`
were written. All eight gates still PASS. Four things the design surfaced that the initial
check had not:

1. **"No plaintext on disk" extends into subprocess arguments.** `tesseract` and
   `pdftoppm` both accept file paths, and the obvious implementation writes a temp file and
   passes its name. Both are therefore invoked with `-` as input and output and fed over
   pipes (research.md D-004). A file path anywhere in an extraction command line is a
   Principle VII violation, and the quickstart checks for one directly.
2. **The browser is part of "no derived copies".** An inline-viewed PDF lands in the
   browser's disk cache by default, which is a decrypted document on disk by another route.
   Responses carry `Cache-Control: no-store` (research.md D-009).
3. **Gate VIII needed reads inside the same transaction as the response.** An audit row
   written after streaming begins cannot roll the read back. The row is written and
   committed before the first byte leaves (research.md D-010).
4. **Gate V's manager-denied step needed a substitute, not a waiver.** This feature has no
   configuration surface. Section F verifies the boundary it does have — no demotion and no
   sensitive-text search, for anyone — rather than recording the principle as N/A.
