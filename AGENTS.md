<!-- SPECKIT START -->
Active feature: **003-property-attachments-ocr** — Property Attachments with Extracted Text.

Read the current plan before making technical decisions:
`specs/003-property-attachments-ocr/plan.md`

It links the specification, the Phase 0 decisions and their rejected alternatives, the
data model, the OpenAPI contract, and the manual verification procedure:

- `specs/003-property-attachments-ocr/spec.md` — 6 stories, 58 requirements, 7 clarifications
- `specs/003-property-attachments-ocr/research.md` — twelve decisions and what each rejected
- `specs/003-property-attachments-ocr/data-model.md` — migrations 0016-0018, states, the
  chunked-encryption layout
- `specs/003-property-attachments-ocr/contracts/openapi.yaml` — the API contract; both the Go
  server interface and the Angular client are generated from it and never hand-edited
- `specs/003-property-attachments-ocr/quickstart.md` — prerequisites and the binding manual
  procedure; section F proves encryption at rest, section G the role boundary

Three rules in this feature are easy to violate and hard to see:
- No plaintext byte on disk, **including a path passed to `tesseract` or `pdftoppm`** —
  both are fed over pipes, never given a filename.
- No derived copy of an attachment anywhere: no thumbnail, no rendered page, and
  `Cache-Control: no-store` so the browser does not cache one either.
- A sensitive attachment's text carries no searchable form at all, so no forgotten query
  predicate can leak it.

Features 001 and 002 are complete; their artifacts remain the reference for sign-in,
sessions, roles, the audit log, Arabic normalisation, properties, and the single-shot
envelope this feature reuses for text:
`specs/001-project-setup-login/plan.md`, `specs/002-properties-crud/plan.md`

Project rules that override defaults live in `.specify/memory/constitution.md` (v1.4.0).
Principle VII was amended twice: v1.2.0 for administrator-designated sensitive fields, and
v1.3.0 for uploader-designated attachment text, derived-content inheritance, and the rule
that nothing leaves the machine. Principle VIII was expanded in v1.4.0: an audit row may
carry before/after values for unencrypted fields only, and a change may be undone as a new
audited change that never edits the log.
<!-- SPECKIT END -->
