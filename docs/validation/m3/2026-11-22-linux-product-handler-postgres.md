# Linux product handler over PostgreSQL — 2026-11-22

## Scope

Bounded #18 continuation (Refs #191) from integration head `0f669a8` (merged PR #487).
Isolated worktree `../amaru-18-next` on branch `feat/18-next`; the shared checkout's
uncommitted changes were not touched.

Changes:

- `httpapi.ProductHandler` assembles the served Project, settings, Approval, Execution and
  Execution Event adapters behind one router bound to one Auth, with one `ProjectAccess`.
  Unserved routes answer `not_found` in the contract's error shape.
- PostgreSQL-tagged `TestPostgresProductHandlerServesDurableState` composes the actual
  Lifecycle, Storage, Schema, Recovery and HTTP owners with it.

No contract, schema, store, migration or dependency change. No entrypoint, connector or
configuration flag is added.

## Evidence

**Unit-tested (fakes, real compact auth):** every dependency is required; every served
pattern reaches its own router entry; the Project, settings, Approval and Execution list
families answer 200 through the assembly; an unknown path, an unserved contract route and a
served path with another method answer JSON `not_found`; the fallback keeps the Host guard.
Every OpenAPI operation is accounted for as served here, served by the HTTP owner or auth,
or explicitly pending.

**Integration-tested (PostgreSQL 16.15, real loopback, actual Lifecycle owners):**

- Two Projects registered with clones each list exactly their own Execution.
- One Project's path cannot address the other's Execution or its Events (`not_found`).
- `cancelExecution` over HTTP commits; the Event page ends in `execution.cancelled` and has a
  `nextCursor`; the other Project's page stays `{"events":[],"nextCursor":0}`.
- Approval list, settings and Project list answer 200; `/workflows` answers `not_found`.
- After stopping and starting fresh owners on the same database (new administrator
  secret), the Execution and Event page bytes are identical, including the derived Event ids.

The Coordinator uses the permissive test rules: `Recovery` wires no trust store, and default
Policy refuses every Step of an untrusted Project.

**Not validated:** a production entrypoint or configured connector; trust, grant, Skill or
context stores in `Recovery`; `decideApproval` and Project attach over PostgreSQL HTTP; a
Worker running the Steps; a browser or SDK client; macOS and Windows.

## Commands and outcomes

Local Linux amd64, Go 1.26.0. These are local results, not CI. PostgreSQL ran in a disposable
Podman container (`docker.io/library/postgres:16-alpine`, 16.15) bound to `127.0.0.1:55491`,
trust authentication confined to that instance.

| Command | Outcome |
|---|---|
| `go test -race -count=10 ./internal/adapter/httpapi` (from `services/platform`) | PASS |
| `AMARU_TEST_PG=… go test -race -tags pg -count=5 -run 'TestPostgresProductHandlerServesDurableState\|TestPostgresHTTPWriteDrains' ./internal/platform` | PASS |
| `AMARU_TEST_PG=… make platform-test-pg` | PASS (exit 0) |
| `make check` | PASS (exit 0) |
| `git diff --check` | clean |

## Known gaps

- `Recovery` constructs the Coordinator without the PostgreSQL trust, grant, Skill and
  context stores. A production composition built on it today would refuse every Step.
- `ProductHandler` has no caller outside tests; the executable and CLI remain pending.
