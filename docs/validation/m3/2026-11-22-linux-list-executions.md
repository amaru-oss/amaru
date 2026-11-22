# Linux listExecutions HTTP — 2026-11-22

## Scope

Bounded #18 continuation (Refs #191) from integration head `b66a0bc` (merged PR #485).
Isolated worktree `../amaru-191-list-executions` on branch `feat/191-list-executions`;
the shared checkout's uncommitted changes were not touched.

Changes:

- `ports.ExecutionStore.ProjectExecutions(project)`: ids of the Project's Executions,
  newest first (creation time, then id, both descending). Fake and PostgreSQL
  implementations; index `executions_by_project (project_id, created_at DESC, id DESC)`
  added to the unreleased execution-scopes migration directly (pre-v1, no upgrade
  path), with `migrations.sum` refreshed.
- `coordinator.ProjectAccess.Executions`: Project authorization before the reads,
  identity recheck after them; an Execution or Step outside the query fails the read.
- `httpapi.ExecutionsHandler` serves `GET /v1/projects/{projectId}/executions`.
- Shared fixture `ExecutionList/valid-platform.json`.
- `make platform-test-pg` installs the SDK's pinned dependencies before running (see
  below).

Not included: pagination (the contract defines none), production startup wiring,
`createExecution`, Execution Event replay.

## Evidence

**Conformance-tested (fake and PostgreSQL 16):** `porttest.ExecutionStore` case with
two Projects, an unscoped Execution, equal creation times ordered by id and a finished
Execution still listed, read through a reopened store; an unknown Project answers an
empty, non-nil list.

**Unit-tested (application boundary, fakes):** the read-authority matrix covers
authorized, revoked, canceled, identity changed and storage error during the store
reads, an Execution of another Project and a Step of another Execution; each refused
read returns no data.

**Integration-tested (fakes, real compact auth, in-process HTTP):** the list for the
seed is exactly the shared fixture, which the Go and TypeScript fixture suites validate
against `ExecutionList.json`. Each Project lists only its own Executions; after a
cancel the list shows the cancelled state and the other Project is unchanged. A query
parameter → `invalid_request`; unknown Project → `not_found`; no credential → 401; a
foreign Host is refused. A failed Execution without its error makes the list answer
`internal` without its id in the body. Route drift: the served patterns are documented
and only `createExecution` and `listExecutionEvents` remain unserved.

**Not validated:** PostgreSQL-backed HTTP for this route, product startup, a real
browser, SDK calls against this route, list size and query plans at volume, macOS and
Windows.

## Earlier `TestProjectsPostgresSDKRecreation` failure explained

The listApprovals record noted an uninvestigated failure of this test in its SDK `write`
phase. Cause, reproduced here: the test runs `node test/postgres.fixture.ts` in
`packages/sdk`, which needs the SDK's installed dependencies. A fresh worktree has none,
and in the earlier run `make check` was installing them concurrently. With
`packages/sdk/node_modules` removed the test fails exactly that way; with it installed
all three phases pass. `make platform-test-pg` now runs `pnpm install --frozen-lockfile`
in `packages/sdk` first; a run from a worktree without dependencies then passed.

## Commands and outcomes

Local Linux amd64. These are local results, not CI. PostgreSQL ran in a disposable
Podman container (`docker.io/library/postgres:16-alpine`) bound to `127.0.0.1:55491`,
trust authentication confined to that instance.

| Command | Outcome |
|---|---|
| `go test -race -count=10 ./internal/adapter/httpapi ./internal/coordinator ./internal/ports/...` (from `services/platform`) | PASS |
| `AMARU_TEST_PG=… go test -tags pg -run 'TestExecutionStoreConformance\|TestMigrations' ./internal/store/...` | PASS (new case included) |
| `AMARU_TEST_PG=… make platform-test-pg` from a worktree without SDK dependencies | First run: SDK fixture passed; `worker/embedded` `TestFailureAndPanicAreReported` timed out after 5 s ("timed out waiting for both Steps to fail"). That test passed 20/20 alone (`-count=20`), and a full rerun passed (exit 0). This slice does not touch the embedded Worker; the timing failure under load was not investigated further. |
| Go/TypeScript fixture parity (`make contracts-ts` steps) | PASS (356 fixtures, identical outcomes) |
| `make check` | PASS (exit 0) |
| `git diff --check` | clean |

## Known gaps

- The list is unbounded; the contract has no pagination or limit.
- Each Execution is read separately (one query per Execution), so the list is not one
  snapshot and costs grow with the Project's history.
- `workflowId`, `providerId`, inputs and outputs are not mapped (see the
  [getExecution record](2026-11-22-linux-execution-http.md)).
