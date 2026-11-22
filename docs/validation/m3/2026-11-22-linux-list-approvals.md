# Linux listApprovals HTTP — 2026-11-22

## Scope

Bounded #18 continuation (Refs #191, #197) from integration head `8373ed9`
(merged PR #482). Isolated worktree `../amaru-191-list-approvals` on branch
`feat/191-list-approvals`; the shared checkout's uncommitted changes were not touched.

Changes:

- `ports.ExecutionStore.ProjectApprovals(project, state)`: one state, newest request
  first (requested time, then id, both descending). Fake and PostgreSQL
  implementations; index `approvals_by_project (project_id, state, requested_at DESC,
  id DESC)` added to the unreleased approvals migration directly (pre-v1, no
  upgrade path), with `migrations.sum` refreshed.
- `coordinator.ProjectAccess.Approvals`: Project authorization before the read,
  identity recheck after it; a row outside the query fails the read.
- `httpapi.ApprovalsHandler` serves `GET /v1/projects/{projectId}/approvals`.
- Shared fixture `ApprovalList/valid-platform-requested.json`.

Not included: pagination (the contract defines none), production startup wiring,
Execution/Event HTTP.

## Evidence

**Conformance-tested (fake and PostgreSQL 16.15):** `porttest.ApprovalStore` case for
two Projects, three states, equal request times ordered by id, a decided Approval
read back with its decision, an unknown Project answering an empty list, through a
reopened store.

**Unit/integration-tested (application boundary, fakes):** the read-authority matrix
covers authorized, revoked, canceled, identity changed and storage error during the
store read, and a store row from another Project; each refused read returns no data.
Two attached Projects list only their own Approvals; an unknown state is
`ErrInvalid` and an unknown Project `ErrNotFound`.

**Integration-tested (fakes, real compact auth, in-process HTTP):** the default list
for the seed is exactly the shared fixture, which the Go and TypeScript fixture suites
validate against `ApprovalList.json`; `?state=requested` answers the same body. Each
Project lists only its own Approvals; after a decision the Approval moves from
`requested` to `approved`; an empty result is `[]`. Unknown, empty and repeated
`state` → `invalid_request`; unknown Project → `not_found`; no session → 401; a
foreign Host is refused. A voided Approval makes `?state=voided` answer `internal`
without its id in the body. Route drift: every documented Approval operation is
served and every served pattern is documented.

**Not validated:** PostgreSQL-backed HTTP, product startup, a real browser, SDK calls
against these routes, query plans at volume, macOS and Windows.

## Commands and outcomes

Local Linux amd64. These are local results, not CI. PostgreSQL ran in a disposable
Podman container (`docker.io/library/postgres:16-alpine`, 16.15) bound to
`127.0.0.1:55491`, trust authentication confined to that instance.

| Command | Outcome |
|---|---|
| `go test -race -count=10 ./internal/adapter/httpapi ./internal/coordinator ./internal/ports/...` (from `services/platform`) | PASS |
| `AMARU_TEST_PG=… go test -tags pg -count=1 -run 'TestApprovalStoreConformance' ./internal/store/...` | PASS (new case included) |
| `AMARU_TEST_PG=… make platform-test-pg` | PASS (exit 0) when run alone. A first run concurrent with `make check` failed `TestProjectsPostgresSDKRecreation` in its SDK `write` phase; that package passed when rerun alone, and the full suite then passed. The collision cause was not investigated further. |
| `make contracts-ts` | PASS (353 fixtures, identical Go/TypeScript outcomes) |
| `make check` | PASS (exit 0; doccheck ok) |
| `git diff --check` | clean |

## Known gaps

- The list is unbounded; the contract has no pagination or limit.
- Voided Approvals remain unrepresentable (no `voidedBy`), so a voided row fails
  the list. Not reachable through public commands today.
- Each Approval with a `definition` item reads its Execution once for the pin.
