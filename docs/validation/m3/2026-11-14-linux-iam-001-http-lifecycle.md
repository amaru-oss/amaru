# IAM-001 HTTP/Auth/discovery lifecycle slice — Linux

Date: 2026-11-14. Runner: Codex, local Fedora Linux 43 x86_64, kernel 7.2.11-100.fc43.x86_64. Machine classification: `systemd-detect-virt` reports `container-other`; physical/full-VM status is unverified. This is local evidence, not hosted CI.

Base: #18 integration `daae1d01a5a2655202e5f1baa8c7afc611dfd968`, verified from the remote ref after PR #392. PR #391 merged to #19 (`feat/19-wp-m3-07-worktrees-configuration-scopes`) and is not an ancestor of this base. Source: this record's commit, excluding documentation-only follow-ups. New additive worktree `/tmp/amaru-92-startup`, branch `feat/92-private-discovery-startup`; existing worktrees and changes preserved.

## Scope and architecture review

Exactly one slice: the concrete HTTP/Auth/private-discovery resource owner in `services/platform/internal/platform/http.go`, its Linux integration tests, health error-response metadata/generated SDK type and affected documentation. Accepted ADR-0034/0060/0061 supply compact security; ADR-0068 supplies listener ownership, readiness and bounded teardown. ADR-0070 remains proposed and unrelated. No predecessor source is copied, so no new migration provenance entry is needed.

Verdict: **sound within this resource-owner scope**. The Platform retains state and authentication; handlers use the existing authorized application boundary. No domain, Coordinator, Worker, Provider, JSON Schema, ErrorCode, infrastructure or dependency changes. OpenAPI Health response metadata and generated SDK types do change: Health documents admission failure (503 with the existing `internal` Error envelope) and its existing Host rejection using the shared default Error response; SDK types are regenerated. This maps accepted ADR-0068 admission semantics without a new operation, error code or health shape. Composition supplies an explicit state directory and authorized product handlers, as it supplies concrete dependencies to existing resource owners. The simpler SDK fixture cannot own production resources: it publishes before handler initialization and lacks aggregate admission, retained ownership through drain and bounded handler joins.

The owner is **IPv4-only** and binds `127.0.0.1:0` inside Start, constructs Auth from the actual address, initializes handlers and reaches Serve's Accept before publishing. Aggregate readiness gates all dispatch. Shutdown cancels upgrade request contexts, closes tracked hijacked connections, bounds ordinary draining to five seconds or the remaining global cleanup budget, then cancels/force-closes requests on expiry. Auth/publication are released only after Serve and admitted handlers join. Cleanup timeout is failure, requiring process exit; an unjoined handler keeps publication/Auth owned rather than allowing replacement startup. Fresh owner recreation rotates credentials. Same-OS-user processes remain inside the accepted compact trust boundary.

The connection registry removes hijacked connections when closed. Product Event adapters must own and join subscriptions within their handler lifetimes; this owner does not invent a subscription API or product Event implementation. Raw upgrade tests are connection-lifecycle evidence only. Canonical per-user path/override selection, CLI `amaru open`, the production root and missing messaging/outbox prerequisites, full product API/Event composition and IPv6/dual-stack publication remain **unimplemented by this slice**. No human decision is needed for this slice; path/override policy must be resolved before canonical bootstrap/CLI discovery can be claimed.

## Commands and outcomes

- `go test -race -count=1 ./internal/platform -run TestHTTP` (from `services/platform`): PASS, exit 0. The first compile identified an incorrect test field name; corrected to the existing `importClaudeSkills` contract. The first race run exposed an unlocked registry read in the test assertion; corrected to lock the assertion. No race suppression or skipped test.
- `go test -race -count=1 ./internal/platform` (from `services/platform`): PASS, exit 0, including existing storage/recovery/execution/watchdog/diagnostic cases and all seven new HTTP tests (plus five startup-failure subtests).
- `pnpm --dir packages/sdk run generate`: PASS, exit 0; generated health Error response type matches OpenAPI.
- `make check` (root): PASS, exit 0 on the final code/contract change. Includes format, documentation, all Go module vet/tests, Windows/macOS cross-vet, Protobuf lint/format/breaking checks, 348 TypeScript contract tests, 346 identical Go/TypeScript fixture outcomes and 48 SDK tests. Cross-vet is not native OS evidence.
- `make docs-check` and `git diff --check`: PASS, exit 0 (standalone final documentation/whitespace verification).

Tool versions: Platform module selects Go 1.27.0 linux/amd64 (root launcher Go 1.26.0); Node v25.7.0; pnpm 11.5.0; buf 1.69.0. PostgreSQL and browser are not used in this slice. Node 22 is not separately validated. Go net/http ownership semantics were checked in the selected toolchain source (`server.go`): Shutdown excludes hijacked connections and does not force handlers to join; the owner tracks/joins those explicitly within its budget.

## Evidence levels

| Behavior | Evidence |
| --- | --- |
| Actual Lane with HTTP/Auth/discovery and existing Project settings handler | **integration-tested**, fake Project/Execution storage, actual loopback requests and Linux private file checks through discovery |
| No admission before aggregate readiness; matching health/discovery metadata | **integration-tested** |
| Administrator issuance, launch/code-free redirect, cookie/session/CSRF settings mutation; missing credentials, hostile Host and Origin-bearing bearer rejection | **integration-tested**; no real browser in this slice |
| Failed handler initialization/panic, unsafe permissions, competing owner publication, prerequisite failure | **integration-tested**; no successful readiness, listener/credential rollback and preservation of earlier owner |
| Restart rotates administrator credentials and rejects old bearer | **integration-tested**, owner recreation, not an OS process crash |
| Ordinary requests survive Start cancellation and drain; upgrades cancel/close; late admission rejected | **integration-tested**, test-only raw upgrade, no product WebSocket protocol |
| Uncooperative handler exceeds shared shutdown budget | **integration-tested**, failure with forced request cancellation/connection closure; publication retained until process exit rather than falsely claiming successful join |
| Unexpected essential serving exit | **integration-tested**, fails lifecycle and removes publication after joins |
| Auth diagnostics | **integration-tested**, captured existing operational middleware output excludes administrator secret, launch code, cookie credential and CSRF token |
| Production startup/CLI/Event transport, browser SDK, dual stack | **not validated / not implemented by this slice** |
| Native macOS/Windows permissions, ACLs, lifecycle | **not validated**; discovery continues to fail closed outside Linux |

Earlier native Firefox/IPv6 adapter evidence remains in the [IPv6 record](2026-11-13-linux-iam-001-ipv6.md); it does not validate this owner. No real agent, paid API or external database is required by these tests. **#92 remains partial**: this resource-owner slice does not complete IAM-001. #18 and M3 acceptance remain incomplete.
