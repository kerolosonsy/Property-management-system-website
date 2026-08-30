<!-- SPECKIT START -->
Active feature: **002-properties-crud** — Properties Register.

Read the current plan before making technical decisions:
`specs/002-properties-crud/plan.md`

It links the specification, the Phase 0 decisions and their rejected alternatives, the
data model, the OpenAPI contract, and the manual verification procedure:

- `specs/002-properties-crud/spec.md` — what the feature must do, with five clarifications
- `specs/002-properties-crud/research.md` — eleven decisions and what each rejected
- `specs/002-properties-crud/data-model.md` — migrations 0009-0014, constraints, privileges
- `specs/002-properties-crud/contracts/openapi.yaml` — the API contract; both the Go
  server interface and the Angular client are generated from it and never hand-edited
- `specs/002-properties-crud/quickstart.md` — setup commands and the binding manual
  verification procedure, section G being the manager-denied-configuration step

Feature 001 (Project Foundation & Login) is complete and its artifacts remain the
reference for sign-in, sessions, roles, the audit log, and Arabic normalisation:
`specs/001-project-setup-login/plan.md`

Project rules that override defaults live in `.specify/memory/constitution.md` (v1.2.0).
Principle VII was amended for this feature: administrators may designate a free-text
custom field sensitive, which encrypts it at rest and removes it from every search.
<!-- SPECKIT END -->
