# Linux Execution HTTP (getExecution, cancelExecution) — 2026-11-22

## Scope

Bounded #18 continuation (Refs #191) from integration head `946b8cb` (merged PRs #483
and #484). Isolated worktree `../amaru-191-execution-http` on branch
`feat/191-execution-http`; the shared checkout's uncommitted changes were not touched.

Changes:

- `Execution.json`: `capability` and `providerId` are optional (owner delegation,
  2026-11-22). `capability` describes the only Step of a single-Step Execution; `steps`
  is authoritative. Changed directly under the pre-v1 rule; the generated SDK types
  were regenerated.
- `httpapi.ExecutionsHandler` serves `GET /v1/projects/{projectId}/executions/{id}`
  and `POST .../cancel` through compact authentication and `ProjectAccess.Execution` /
  `ProjectAccess.Cancel`. The Approval error mapper is now shared as `accessError`.
- Shared fixtures `Execution/valid-platform-workflow.json` (two-step Workflow run, no
  top-level capability) and `Execution/valid-platform-single-step.json`.

Not included: `listExecutions`, `createExecution`, `listExecutionEvents`, production
startup wiring, `workflowId` mapping (domain slug vs contract `WF-<digits>`), Agent
snapshot mapping.

## Evidence

**Integration-tested (fakes, real compact auth, in-process HTTP):**

- The seeded Workflow run and single-Step Execution are exactly the shared fixtures,
  which the Go and TypeScript fixture suites validate against `Execution.json`.
- A browser cancel (session cookie, Origin, CSRF) ends both Steps and the Execution;
  a same-key replay returns the identical body with no store write; the same key
  with another reason and a new key on the ended Execution are `conflict` with no
  write; the other Project's Execution is unchanged.
- Foreign and missing Executions give identical `not_found` for read and cancel,
  with no write.
- Unknown field, missing/empty/long key, reason over 4000 characters, two documents,
  non-object, oversized, malformed, trailing value, wrong JSON types and wrong media
  type are `invalid_request` with no write; no credential is 401.
- Missing CSRF, foreign Origin and foreign Host are refused with no write.
- All shared `CancelExecution` fixtures: valid → 200, invalid → 400.
- Route drift: both served patterns are documented; the three unserved Execution
  operations are listed explicitly, so a newly documented or newly served route fails.

**Unit-tested:** the mapper refuses (`internal`) a failed Execution or Step without an
error, an incomplete error, an ended record without `endedAt`, a queued Execution with
`startedAt`, unknown states, a Step from another Execution, a refused Step without a
reason, an invalid Capability id, an Agent snapshot and a Step missing from the pinned
definition.

**Not validated:** PostgreSQL-backed HTTP (no store change in this slice), product
startup, a real browser, SDK calls against these routes, a cancel that reaches a Worker
holding a Step, macOS and Windows.

## Commands and outcomes

Local Linux amd64. These are local results, not CI.

| Command | Outcome |
|---|---|
| `go test -race -count=10 ./internal/adapter/httpapi ./internal/coordinator` (from `services/platform`) | PASS |
| Go/TypeScript fixture parity (`make contracts-ts` steps) | PASS (355 fixtures, identical outcomes) |
| `make check` | PASS (exit 0; doccheck ok) |
| `git diff --check` | clean |

## Known gaps

- `workflowId` is not mapped until the Workflow id formats are reconciled.
- `providerId`, inputs, outputs and Work Item links are not recorded on the domain
  Execution.
- Replay returns the current Execution state, not a snapshot taken when the command
  first ran.
