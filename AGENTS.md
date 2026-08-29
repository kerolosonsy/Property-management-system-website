<!-- SPECKIT START -->
Active feature: **001-project-setup-login** — Project Foundation & Login.

Read the current plan before making technical decisions:
`specs/001-project-setup-login/plan.md`

It links the specification, the Phase 0 decisions and their rejected alternatives, the
data model, the OpenAPI contract, and the manual verification procedure:

- `specs/001-project-setup-login/spec.md` — what the feature must do
- `specs/001-project-setup-login/research.md` — why each technical choice was made
- `specs/001-project-setup-login/data-model.md` — tables, constraints, privileges
- `specs/001-project-setup-login/contracts/openapi.yaml` — the API contract; both the Go
  server interface and the Angular client are generated from it and never hand-edited
- `specs/001-project-setup-login/quickstart.md` — setup commands and the binding manual
  verification procedure

Project rules that override defaults live in `.specify/memory/constitution.md` (v1.1.0).
<!-- SPECKIT END -->
