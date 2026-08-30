# Implementation Plan: [FEATURE]

**Branch**: `[###-feature-name]` | **Date**: [DATE] | **Spec**: [link]

**Input**: Feature specification from `/specs/[###-feature-name]/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

[Extract from feature spec: primary requirement + technical approach from research]

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: [e.g., Python 3.11, Swift 5.9, Rust 1.75 or NEEDS CLARIFICATION]

**Primary Dependencies**: [e.g., FastAPI, UIKit, LLVM or NEEDS CLARIFICATION]

**Storage**: [if applicable, e.g., PostgreSQL, CoreData, files or N/A]

**Testing**: [e.g., pytest, XCTest, cargo test or NEEDS CLARIFICATION]

**Target Platform**: [e.g., Linux server, iOS 15+, WASM or NEEDS CLARIFICATION]

**Project Type**: [e.g., library/cli/web-service/mobile-app/compiler/desktop-app or NEEDS CLARIFICATION]

**Performance Goals**: [domain-specific, e.g., 1000 req/s, 10k lines/sec, 60 fps or NEEDS CLARIFICATION]

**Constraints**: [domain-specific, e.g., <200ms p95, <100MB memory, offline-capable or NEEDS CLARIFICATION]

**Scale/Scope**: [domain-specific, e.g., 10k users, 1M LOC, 50 screens or NEEDS CLARIFICATION]

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Derived from `.specify/memory/constitution.md` v1.2.0. Mark each gate PASS, FAIL,
or N/A with a one-line justification. A FAIL blocks the phase until resolved or
recorded in Complexity Tracking below.

- [ ] **I. Authorization** — Every new endpoint has an explicit server-side role
      check (admin vs manager); manager is denied all configuration access; no
      route relies on an Angular guard for enforcement.
- [ ] **II. Arabic RTL** — All new UI strings are Arabic; layout uses logical CSS
      properties; any imported `.dc.html` design has a corresponding RTL
      conversion task.
- [ ] **III. Contract-First** — Every new or changed endpoint is defined in
      `contracts/` OpenAPI before implementation; the Angular client is generated
      from it.
- [ ] **IV. Data Integrity** — Relationships are real foreign keys; schema change
      ships as a forward-only migration; money uses integer minor units or
      NUMERIC (never float); timestamps are UTC `timestamptz`.
- [ ] **V. Manual Verification** — `quickstart.md` contains a concrete,
      step-by-step verification procedure, including a manager-denied-config step
      where configuration is touched.
- [ ] **VI. Simplicity** — No speculative abstraction, no unrequested
      configurability, no new dependency this feature does not need.
- [ ] **VII. Encryption** — Uploads are AES-256-GCM envelope-encrypted before
      touching disk (no plaintext temp file); attachments served only via an
      authorized decrypting handler; national ID / passport / bank account / IBAN
      columns encrypted; any administrator-designated sensitive field is free-text
      only, fixed once values exist, envelope-encrypted, and excluded from search,
      sort, filter, and report; no secret, key, or decrypted value reachable by a
      log; TLS on every listener including local.
- [ ] **VIII. Audit** — Every create/update/delete on business records and every
      attachment download writes an append-only audit row (actor, role, action,
      entity, UTC timestamp, IP) in the same transaction as the change.

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete layout
  for this feature. Delete unused options and expand the chosen structure with
  real paths (e.g., apps/admin, packages/something). The delivered plan must
  not include Option labels.
-->

```text
# [REMOVE IF UNUSED] Option 1: Single project (DEFAULT)
src/
├── models/
├── services/
├── cli/
└── lib/

tests/
├── contract/
├── integration/
└── unit/

# [REMOVE IF UNUSED] Option 2: Web application (when "frontend" + "backend" detected)
backend/
├── src/
│   ├── models/
│   ├── services/
│   └── api/
└── tests/

frontend/
├── src/
│   ├── components/
│   ├── pages/
│   └── services/
└── tests/

# [REMOVE IF UNUSED] Option 3: Mobile + API (when "iOS/Android" detected)
api/
└── [same as backend above]

ios/ or android/
└── [platform-specific structure: feature modules, UI flows, platform tests]
```

**Structure Decision**: [Document the selected structure and reference the real
directories captured above]

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
