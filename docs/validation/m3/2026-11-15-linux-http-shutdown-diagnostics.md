# HTTP shutdown diagnostics — Linux, 2026-11-15

## Scope

Bounded #170 integration verification after PR #420 merged into #18 at
`8c3088f`. Native blockers #83/#84 and inherited #15 were verified closed.
Work uses an isolated checkout; shared uncommitted work is preserved.
No runtime, contract, dependency or architecture decision changes.

`TestHTTPBoundedShutdownCancelsAndPreservesPublicationUntilJoin` now captures
both HTTP and lifecycle JSON during the existing bounded-shutdown scenario.
The real loopback listener authenticates a native request, shutdown expires
its cleanup budget, and the handler observes cancellation but deliberately
waits for release. Discovery remains published until the handler joins.
The test then releases the handler and cleans up the abandoned publication.
This is test cleanup, not a supported recovery after a production timeout.

After both producers join, the test closes and drains lifecycle diagnostics
through `ConsumeLifecycle` into the same sink as HTTP. It checks exactly one
HTTP cancellation record with `request_canceled`, retained status 200 and the
handler's trusted correlation, accounts for every queued lifecycle record,
and rejects administrator credential, query and forged correlation fixtures
in the combined output. Status 200 here is the wrapper's default captured
status; forced connection closure does not promise a response to the client.

## Verification

- **Integration-tested:** real HTTP/Auth/discovery/Lane shutdown with an
  in-process test handler and the operational JSON consumer. No real Provider.
- Removing only PR #420's cancellation classification causes the focused
  regression to fail with `shutdown cancellation diagnostic missing or lost
  request correlation` (exit 1). Source restored immediately afterward.
- `go test -race -count=20 -run '^TestHTTP(BoundedShutdownCancelsAndPreservesPublicationUntilJoin|DiscoveryAdmissionAndProductAuthentication)$' ./internal/platform`: PASS, exit 0, both tests repeated 20 times, no skips.
- `go test -race -count=1 ./internal/adapter/operational ./internal/arch`:
  PASS, exit 0, no skips.
- `make check`: PASS, exit 0, including 348 TypeScript contract tests,
  346 identical Go/TypeScript fixture outcomes, 48 SDK tests, Platform tests,
  documentation checks and native/cross-platform vet targets.
- `git diff --check`: PASS, exit 0.
- `trail check --base 8c3088f`: exit 1, three reviewed lexical conflicts:
  the README consumer description does not change cancellation ordering
  (`l-73c141f1`) or SQL panic handling (`l-2c82b2cb`). The HTTP client now
  uses its own transport with `DisableKeepAlives`, following `l-9dd96534`;
  it does not increase the shutdown budget or add a second shutdown.
  Diagnostic draining remains after both producer joins. These conflicts
  do not identify a semantic violation; Trail is not reported as passing.

Environment: Linux 7.2.11-100.fc43.x86_64, amd64; Go 1.27.0 selected by
the Platform module, Node v25.7.0, pnpm 11.5.0. Run by Codex for @rluders.
Logs: `/tmp/amaru-after420-check.log` (local artifact, not committed).
An initial edit command used the wrong directory and made no changes;
its test attempt also failed on sandbox cache access. Corrected commands
used the module directory and authorized cache/loopback access.

## Limits

Local Linux evidence only, not hosted CI. No PostgreSQL runtime, native
macOS/Windows, production entrypoint, full stdout/stderr capture or real
Provider validation. Cancellation records describe request lifetime and do
not prove command rollback or durability. Production bootstrap and bounded
consumer joining remain outstanding; #170, #18 and M3 stay open.
