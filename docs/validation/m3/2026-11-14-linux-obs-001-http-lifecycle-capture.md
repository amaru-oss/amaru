# OBS-001 HTTP/Lifecycle diagnostic composition — Linux

Date: 2026-11-14. Runner: Codex, local Linux x86_64, kernel
7.2.11-100.fc43.x86_64. `systemd-detect-virt` reports `container-other`;
physical/full-VM status is unverified. Local results are not hosted CI.

Base: #18 integration `feat/18-wp-m3-06-public-api-events` at
`af70f5b4edd7a3653e4d3071d44f0b6e97821b40`, refreshed after merged #393.
Source: this record's commit. Reused clean `/tmp/amaru-170-lifecycle` on new
branch `test/170-http-lifecycle-capture`; the old #390 branch is preserved and
is an ancestor of the refreshed base. No worktree was created or removed.
Other #170 worktrees were inspected: the original/Worker branches retain
untracked Trail files, product/SDK branches retain untracked Trail caches, and
`/tmp/amaru-m3-after390` retains its Trail cache. None were reset, cleaned or
changed. The initial checkout `/tmp/amaru-124-api` and shared root were preserved.

## Refreshed issue, PR and dependency state

#170 and #18 remain open; native #170 blockers #83/#84 and inherited #18 blocker
#15 are closed. No open PRs were observed at selection. #92/#191/#219 remain
open, and the open M3 issue list still has 25 entries. No acceptance checkbox,
issue status or Project board was changed. Historical issue-body blockers are
not current native dependency state.

All PRs #368–#393 are merged, with these distinct destinations/scopes:

| PRs | Destination and relevant work |
| --- | --- |
| #368–#369 | #18: SDK Project and rejected-auth log capture, using the test-only auth server |
| #370–#371 | #18: PostgreSQL SDK isolation and rejected attachment rollback |
| #372–#374 | #18: session-bound CSRF, safe sink panic containment, committed Workflow dispatch isolation |
| #375–#377 | #19: Evidence identity, Worktree alias rejection, Step attribution |
| #378 | #18: Project slot isolation |
| #379 | main: M4 self-development marker, not M3 completion |
| #380–#381 | #18: safe response I/O diagnostics and IPv6 auth adapter evidence |
| #382–#387 | #19: ADR-0069 proposal/owner acceptance, bounded Usage capture, gaps/replay and consistent Evidence reads |
| #388–#390 | #18: stale Agent edit regression, queue/logging assessment, fake-stage Lifecycle diagnostic composition |
| #391 | #19: malformed audit page rejection |
| #392 | #18: proposed ADR-0070 Work contract; not authorization to implement it |
| #393 | #18: actual HTTP/Auth/discovery lifecycle owner |

The #19 changes are not assumed to exist on this #18 base. #394 subsequently
merged a partial multi-Step Evidence walkthrough to #19; it does not implement
product bootstrap here. Agent authority remains deferred and ADR-0070 remains
proposed. No other issue's blocked implementation is pulled into this change.

## Selected bounded gap and ownership assessment

#390 exercised the actual Lifecycle wrapper with fake stages, not real HTTP.
#393 exercises real HTTP/Auth/discovery and Project settings, but its log checks
only searched for an HTTP operation and missing generated credentials; Lifecycle
diagnostics were disabled. Neither demonstrated JSON/correlation consistency
through both owners and their shared logger on the successful/rejected path.

This change extends `TestHTTPDiscoveryAdmissionAndProductAuthentication` in
`services/platform/internal/platform/http_linux_test.go`. The test composition
caller supplies the real HTTP owner and Lifecycle with the existing logger and a
bounded diagnostic queue. It closes/drains that queue only after `Run` and the
HTTP handlers have joined; callbacks never perform sink IO. The drain supplies
trusted process identity, while HTTP creates independent trusted request identity.
A test-only observation around the existing authorized SettingsHandler verifies
that the successful mutation's context correlation matches its emitted record.
Rejected requests never reach that handler. Existing authentication and persisted
settings checks remain in place.

All captured lines in the shared diagnostic sink must parse as JSON with the
existing exact field set, timestamp and Instance/correlation fields. Ten HTTP
records cover readiness rejection (503), health/session/mutation success (200),
issuance (201), code exchange (303), missing/invalid/mixed credentials (401) and
Host rejection (403). Status counts and outcomes must match, request correlations
must be nonempty/unique, and Lifecycle records must retain the trusted drain
identity without raw attributes. Intentional lifecycle cancellation must still
complete successfully and remove discovery/credentials after joins.

Fixture checks exclude generated administrator, launch, cookie and CSRF values,
plus opaque bearer/cookie/query/body and forged correlation-header fixtures.
Opaque values are intentionally unregistered: HTTP must omit request material
rather than depend on known-value redaction for authentication secrets. Assertions
never print captured lines, request material or generated credentials on failure.

Existing accepted ADR-0034/0060/0061/0062/0068 and the existing logger define all
behavior exercised here. No logging labels, correlation semantics, contracts,
production wiring, dependency or infrastructure is added. Scope reaches one
existing Linux test, this record and links in the Platform README/validation guide;
no production callers, exports, fixtures or configuration change. The shared test
HTTP client helper also uses a request-owned non-reusing transport; its only callers
are this test and `TestHTTPRestartRotatesCredentials`, checked before editing.

**The production entrypoint and diagnostic consumer still do not exist.** This
slice exercises actual product-facing resource owners with a test-owned consumer;
it is not actual production-entrypoint acceptance. Creating a standalone root
solely to call this logging evidence complete would bypass missing concrete
storage/messaging/Provider/product assembly and exceed this slice's guardrails.
No new human architecture decision is required for these existing-owner checks.

## Validation

Commands run from `services/platform` unless noted, using
`GOCACHE=/tmp/amaru-170-lifecycle-cache`:

- `go test -race -count=1 -v ./internal/platform -run '^TestHTTPDiscoveryAdmissionAndProductAuthentication$'`:
  PASS, exit 0 on the first extension. An accompanying test-edit command used a
  root-relative path from the module and failed before that run; corrected.
- `go test -race -count=20 -run '^TestHTTPDiscoveryAdmissionAndProductAuthentication$' ./internal/platform`:
  PASS, exit 0 on the first extension, 20 executions.
- `go test -race -count=1 -v ./internal/platform ./internal/adapter/operational`:
  PASS, exit 0 on the first extension, 47 top-level tests, no skips.
- Initial root `make check`: FAIL, exit 2 because the newly added links preceded
  creation of this record. The record was added before the final rerun.
- First final root `make check`: PASS, exit 0 (348 TypeScript contract tests,
  346 matching fixture outcomes, 48 SDK tests and all Platform checks). A separate
  repeated race run nevertheless FAILED on shutdown; its shell invocation ran the
  affected suites afterward, so that invocation's last exit 0 did not establish
  that the preceding repeated command passed. The log was inspected separately.
- Standalone repeated race reproduction: FAIL, exit 1, six shutdown failures in
  20 runs. Temporary secret-free connection-state instrumentation reproduced
  the failure and showed `new:1 idle:1 closed:1` at shutdown entry, zero admitted
  handlers after forced cleanup and deadline expiry. Selected Go 1.27.0 source
  `net/http/server.go:3303` confirms `closeIdleConns` waits over five seconds before
  treating a StateNew connection as idle. The test helper's shared default
  transport can leave speculative dials when a preceding response closes.
- Temporary production/test instrumentation was removed in full. The helper now
  gives each call its own non-reusing transport, eliminating speculative reuse;
  production code and the one-second cleanup budget are unchanged. This does not
  relax production admission, request drain or timeout behavior.
- Final `go test -race -count=100 -run '^TestHTTPDiscoveryAdmissionAndProductAuthentication$' ./internal/platform`:
  PASS, exit 0, 100 executions, unchanged cleanup budget.
- Final `go test -race -count=1 -v ./internal/platform ./internal/adapter/operational`:
  PASS, exit 0, 47 top-level tests, no skips, including the restart-rotation caller.
- Final root `make check`: PASS, exit 0 after the client fix. Format, docs,
  all Go tests/vet, cross-OS and PostgreSQL-tag vet, Protobuf checks, 348 TypeScript
  contract tests, 346 identical parity fixtures and 48 SDK tests passed.
  Cross-vet is not native OS evidence and tagged vet is not database evidence.
- Final root `make docs-check` and `git diff --check`: PASS, exit 0 after this
  record's outcome update. Logs remain under `/tmp/amaru-170-http-capture-*`.

The connection-state cause and rejected timeout workaround are recorded in the
committed Trail learning, alongside this change; no raw diagnostic payloads were
retained. No production file differs from the base.

Tool versions: root Go launcher 1.26.0, Platform module selects Go 1.27.0;
Node v25.7.0; pnpm 11.5.0. No browser, PostgreSQL runtime or real Provider is used.

## Evidence levels and remaining scope

Successful authorized settings mutation, rejected requests, aggregate admission,
auth/discovery ownership, safe JSON/correlation and post-join diagnostic drain
are **integration-tested** with real loopback/Lane and fake Project/Execution
stores. Generated credential absence and opaque request omission are captured-sink
evidence. This is not full process stdout/stderr capture; net/http raw diagnostics
remain discarded by the existing owner. Returned Lifecycle errors are not printed.

Production entrypoint/consumer, complete command/Worker/channel composition,
Provider failure/cancellation through production assembly, process exit/restart,
PostgreSQL/NATS runtime, product Event transport, native macOS/Windows and real
Provider diagnostics are **not validated by this slice**. No additional browser
validation is claimed. #170, #92, #18 and M3 remain partial/open. Review and merge
are human-owned; this change stops at an unmerged PR.
