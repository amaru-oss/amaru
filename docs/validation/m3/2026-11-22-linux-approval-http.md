# Linux Approval HTTP adapter — 2026-11-22

## Scope

Bounded #18 continuation (Refs #191, #197) from integration head `7fd6898`
(merged PR #481). Isolated worktree `../amaru-191-approval-http` on branch
`feat/191-approval-http`; the shared checkout's uncommitted changes were not touched.

Owner decisions (2026-11-22 planning session): serve `getApproval` and
`decideApproval` next; extend `EvidenceItem.json` with the kinds the Platform
records instead of changing the domain; generate ULID Approval ids instead of
relaxing `Ulid.json`.

Changes:

- Coordinator default Approval ids are ULIDs (stdlib, `crypto/rand`).
- Contract: `EvidenceItem.json` gains `step_inputs`, `skill`, `agent` and
  `context` variants (Digest-form hashes); `DecideApproval.rationale` and
  `ApprovalDecisionRecord.rationale` are bounded at 1000. Pre-v1, no shim.
  Fixtures added; SDK types regenerated.
- `httpapi.ApprovalsHandler` for the two operations behind compact auth.

Not included: `listApprovals`, production startup wiring, Execution/Event HTTP.

## Evidence

**Unit-tested:** ULID format, time prefix (ULID specification example) and
uniqueness; `wireApproval` refuses voided, approved-without-decision,
uncommitted/foreign/missing definition pins, unknown Evidence kinds and an
agent requester without a sponsor.

**Integration-tested (fakes, real compact auth, in-process HTTP):** a seeded
`git.push` Approval read returns exactly the shared fixture
`Approval/valid-platform-requested.json`, which the Go and TypeScript fixture
suites validate against `Approval.json`. Foreign-Project and missing ids give
identical `not_found` for read and decision. A browser-session approval records
the session identity as approver; same-key replay is byte-identical, and a later
read matches. Conflicting key reuse, a second decision, a stale digest → `conflict`.
Missing rejection rationale, oversized rationale, unknown fields, missing key,
bad outcome or digest, two documents, malformed JSON, wrong media type and
oversized body → `invalid_request`. Missing session, CSRF, wrong CSRF, a foreign
Origin and a foreign Host are refused. Every refusal leaves the stored Approval
unchanged. The shared `DecideApproval` fixtures replay through the handler.
Route drift: both served patterns exist in `openapi.yaml`.

**Not validated:** PostgreSQL-backed HTTP, product startup, a real browser,
SDK calls against these routes, macOS and Windows.

## Commands and outcomes

Local Linux amd64. These are local results, not CI.

| Command | Outcome |
|---|---|
| `go test -race -count=10 ./internal/adapter/httpapi ./internal/coordinator` (from `services/platform`) | PASS |
| `make contracts-ts` | PASS (exit 0; Go/TS parity identical) |
| `make sdk-ts` (in `make check`) | PASS (48 SDK tests; generated types current) |
| `make check` | PASS (exit 0; 354 TS contract tests, 352 fixtures with identical Go/TS outcomes, doccheck ok) |
| `git diff --check` | clean |

## Known gaps

- `MaxRationale` counts bytes; the contract's `maxLength` counts characters.
  Multi-byte text near 1000 characters can be refused as `invalid_request`.
- Voided Approvals are not served (the contract requires `voidedBy`, which is
  not recorded); the adapter answers `internal`. Not reachable today: the
  Platform does not void Approvals yet (POLICY_AND_APPROVAL_SHAPES REV-003 note).
- A `definition` item from an uncommitted Workflow submission is not
  representable; only internal trusted Submit produces one.
