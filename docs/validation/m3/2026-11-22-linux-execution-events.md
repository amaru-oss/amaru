# Linux listExecutionEvents HTTP — 2026-11-22

## Scope

Bounded #18 continuation (Refs #191) from integration head `c0a65ad` (merged PR #486).
Isolated worktree `../amaru-191-execution-events` on branch `feat/191-execution-events`;
the shared checkout's uncommitted changes were not touched.

Owner decisions (2026-11-22), applied directly before v1:

- The public event `id` is derived, not stored: event time milliseconds plus 80 bits of
  SHA-256 over the Project id and cursor, in ULID text form.
- Journal kinds without a faithful public shape are omitted; `nextCursor` still advances.
- `EventList` gains an optional `resync`; `listExecutionEvents` caps `limit` at 1000.

Changes:

- Contract: `openapi.yaml` (`limit` maximum 1000, omission and resync described),
  `events/EventList.json` (`resync`, with no events and no `nextCursor` alongside it),
  regenerated SDK types, fixtures `EventList/valid-resync.json`,
  `invalid-resync-with-events.json` and `invalid-resync-with-cursor.json`.
- `httpapi.ExecutionEventsHandler` serves
  `GET /v1/projects/{projectId}/executions/{id}/events` on `ProjectAccess.Events`, with a
  trusted Event stream and replay window.
- Shared fixtures `EventList/valid-platform-cancelled.json` and
  `EventList/valid-platform-failed.json` hold the exact responses for the test seed.

No store, migration, application-boundary or dependency change. Not included: Project-wide
replay, the WebSocket stream, production startup wiring.

## Evidence

**Integration-tested (fakes, real compact auth, in-process HTTP):**

- An Execution with no journal entries answers `{"events":[],"nextCursor":0}`.
- After a real cancel through `cancelExecution`, the list is exactly
  `valid-platform-cancelled.json`: the cancel request (cursor 1) is omitted; two
  `execution.step` cancelled entries and `execution.cancelled` follow.
- A recorded failed run (lease, start, restart retry, Step failure, Execution failure) is
  exactly `valid-platform-failed.json`: the retry (cursor 3) is omitted; both failures carry
  the recorded error. A second read returns identical bytes, including ids.
- Paging with `limit=2` and `after` returns cursors 1–2, then 4 (skipping the omitted 3),
  then 5, then an empty page at the high-water mark.
- `after=99` answers `resync` (`cursor_ahead`, `resumeFrom` 5) with no events and no
  `nextCursor`.
- Another Project's Execution and an unknown Execution return identical `not_found`; the
  other Project's id and Execution do not appear in a Project's list.
- `limit` 0, 1001 or non-numeric, `after` negative, signed or empty, a repeated parameter
  and any other parameter → `invalid_request`; unknown Project → `not_found`; no
  credential → 401; a foreign Host is refused.
- An unknown kind, a failure entry for a Step that is not failed, an entry naming another
  Execution's Step and an entry without a time each answer `internal` without the id.
- The derived id matches the ULID text form, is stable, differs by Project and cursor, and
  orders by time; a zero or pre-1970 time is refused.
- Route drift: only `createExecution` remains unserved among documented Execution routes.

**Contract:** the new and changed fixtures pass the Go and TypeScript suites with identical
outcomes.

**Not validated:** PostgreSQL-backed HTTP for this route (the PostgreSQL replay core itself
is covered by its existing conformance suite, not rerun here), a real cancel or failure
driven through a Worker, product startup, a browser, SDK calls against this route, the
expired-window and `history_unavailable` resync paths over HTTP (covered only by the replay
core's own tests), macOS and Windows.

## Commands and outcomes

Local Linux amd64. These are local results, not CI.

| Command | Outcome |
|---|---|
| `go test -race -count=10 ./internal/adapter/httpapi` (from `services/platform`) | PASS |
| `go vet ./internal/adapter/httpapi` | clean |
| `go test ./...` (from `packages/contracts`) | PASS |
| Go/TypeScript fixture parity (`make contracts-ts` steps) | PASS (361 fixtures, identical outcomes) |
| `pnpm run generate` (from `packages/sdk`) | regenerated `schema.ts` committed |
| `make check` | PASS (exit 0) |
| `git diff --check` | clean |

## Known gaps

- Omitted kinds leave visible `seq` gaps; a client cannot see retries, deferrals, cancel
  requests, stale results or Approval decisions through this route.
- The derived id is unique per Project cursor and stable, but has no random component.
- A failure's error is read from the current record after the page, not from the journal
  entry; this is exact only because failed Steps and Executions are terminal.
