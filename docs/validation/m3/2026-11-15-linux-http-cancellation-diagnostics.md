# HTTP cancellation diagnostics — Linux, 2026-11-15

## Selection and scope

PR #419 was verified merged into `feat/18-wp-m3-06-public-api-events`
at `7977c6b6a83e41df71f86e85d7f47b105558bc27`. The isolated implementation
checkout was synced to that base; shared-checkout work was preserved.
Selected bounded follow-up: #170 (OBS-001), whose native blockers #83/#84
and inherited #15 were verified closed through GitHub. #170, #18 and M3
remain open for complete product instrumentation and acceptance.

## Behavior and reproduction

Before this correction, a handler returning with a canceled request context
and a 200 or 204 status emitted `succeeded`. The deterministic regression
failed in all four cancellation/deadline and 200/204 combinations on the
unchanged implementation (exit 1). Rejected-response cases already passed.

The existing operational HTTP wrapper now records `canceled` with a static
`request_canceled` or `request_deadline_exceeded` cause for those requests.
It retains captured status, trusted Instance/correlation identity, rejected
response precedence, response I/O failure precedence and safe panic handling.
It does not read `context.Cause`, request data or response contents. A request
cancellation does not establish that a command was rolled back or uncommitted.
There is no retry or change to handler execution, public contracts, storage,
dependencies or lifecycle ownership; no new architecture decision is needed.

## Evidence

- **Unit-tested:** `TestHTTPCancellationDiagnostics` covers cancellation and
  deadline expiration with 200/204/403/500 status capture, unchanged response
  status, trusted identity, and omission of an unregistered private custom
  cancellation/deadline cause and query fixture.
- **Integration-tested:** `TestHTTPClientDisconnectDiagnostic` uses a real
  loopback server and client cancellation after handler entry. It joins the
  server before reading output, verifies the canceled JSON record and checks
  that raw request data and server error output are absent.
- `go test -race -count=20 ./internal/adapter/operational`: PASS, exit 0,
  including the existing sink/panic/upgrade/response-I/O and lifecycle tests;
  no skipped tests.
- `go test -race -count=20 -run '^TestHTTPDiscoveryAdmissionAndProductAuthentication$' ./internal/platform`:
  PASS, exit 0, no skips. Existing composed HTTP/Auth/Lane diagnostics remain green.
- `make check`: PASS, exit 0, including docs-check, contract/SDK tests and
  Platform checks. PostgreSQL and non-Linux paths are compile/vet checks only.
- `git diff --check`: PASS, exit 0. `trail build`: PASS, exit 0.
- `trail check --base 7977c6b6a83e41df71f86e85d7f47b105558bc27`:
  PASS, exit 0, no conflicts. Advisory lexical matches concern existing
  authorization, storage, lifecycle and test rules; those implementations are
  unchanged. The new client request is canceled and the test server is joined
  before examining diagnostics.

## Limits

These are local Linux results, not hosted CI. No native non-Linux run,
PostgreSQL runtime, production entrypoint, real Provider or complete product
stdout/stderr capture is established here. Cancellation is sampled when the
handler returns; cancellation after that sample cannot change the record.
The wrapper cannot infer command durability from HTTP status or context.
