# OBS-001 HTTP upgrade status capture

Date: 2026-11-14. Host: Linux amd64, Go 1.26.0. Base: #18 integration
`4136c1645cfb992f9b1d18328dbca260b4b0f149` (merged PR #413).

## Scope and selection

Implemented-and-verified partial #170 slice under accepted
[ADR-0059](../../architecture/adr/0059-adapt-the-prototype-http-handlers-to-the-platform-public-api.md)
and [ADR-0060](../../architecture/adr/0060-adapt-the-prototype-auth-and-discovery-for-compact-installs.md).
The shared operational HTTP wrapper now treats `101 Switching Protocols` as a
final status, matching Go 1.26.0 `net/http/server.go`'s `WriteHeader` implementation.
Previously, the client received 101 while the JSON diagnostic reported 200, or
204 if the handler attempted a later header. Other informational responses remain
non-final. No public route, contract, dependency or prototype code is introduced.

Origin and all open M3 native dependency edges were refreshed before selection.
The PR queue was empty. #170's native #83/#84 and inherited #15 are closed;
the designated base remains `feat/18-wp-m3-06-public-api-events`. #92/#191/#219
also have closed native prerequisites but retain composed acceptance gates.
#16/#17/#19 have closed #15 prerequisites and separate integration queues;
#124/#154 have closed native prerequisites on #19. #197 waits for #124, #125
for #197, and Provider-dependent work still waits for #80/#81/#82; #189/#190
also depend on #113. #188 has closed #135 but retains its audit acceptance gate.
#20 waits for #16/#17/#18/#19. These facts do not establish readiness for every
slice. ADR-0070 on #18 and ADR-0071 on #19 remain proposed and unimplemented.
#170/#191/#18 and M3 remain open.

## Evidence and commands

All commands used `TMPDIR=/home/rluders/amaru-m3-170-tmp`; logs are in that
external directory, with no task writes to `/tmp` or cleanup of untracked data.

- Before the fix: `go test -C services/platform -count=1 -run '^TestHTTPInformationalAndUpgradeStatus$' ./internal/adapter/operational`
  — exit 1; three upgrade cases reproduced incorrect diagnostics. Log:
  `upgrade-status-before.log`.
- After the fix: `go test -C services/platform -race -count=20 -run '^TestHTTPInformationalAndUpgradeStatus$' ./internal/adapter/operational`
  — exit 0; all six cases passed on each repetition. Log: `upgrade-status-after.log`.
- `go test -C services/platform -race -count=1 ./internal/adapter/operational ./internal/platform ./internal/arch`
  — exit 0; all three packages passed. Log: `upgrade-status-composition-race.log`.
- `make check` — exit 0; 346 identical Go/TypeScript fixture outcomes and 48 SDK
  tests passed, along with all other required targets. Complete log:
  `upgrade-status-make-check.log`.
- Final `make docs-check` and `git diff --check` — exit 0. The initial standalone
  docs attempt was denied by the filesystem sandbox; the authorized retry caught
  an incorrect ADR-0060 link in this new record, corrected before the final pass.
- `trail check --base origin/feat/18-wp-m3-06-public-api-events` — exit 0, no
  conflicts. The staged five-file check emitted 16 lexical advisories, reviewed:
  this change alters no SQL/schema, authorization, receive-pump, scheduling,
  connection-ownership or shutdown behavior; it preserves external temp storage,
  the no-legacy rule and Linux-first evidence limits. Log: `upgrade-status-trail-check.log`.

The regression is **integration-tested** with a real loopback HTTP server:
direct 101, 103 then 101, attempted header after 101, 103 then 204, and implicit
200 through Write or Flush. Each case parses the JSON diagnostic and checks
wire/log status agreement, success, correlation and absence of query/body secret
fixtures. Existing raw-hijack, failure and panic controls also passed in the
operational suite. This is local evidence, not hosted CI or a real Provider run.

## Limits

Full product bootstrap, Event transport, complete channel capture and #170
acceptance remain open. A selected HTTP status does not prove response delivery
or successful protocol use after upgrade. PostgreSQL, real Providers, native
macOS/Windows runtime and a browser were not run for this status-only change;
cross-platform vet in `make check` is not native runtime evidence. The earlier
Worker timeout recorded for PR #413 is not represented as fixed by this slice.

## Informational header after a final response

The live HTTP regression sends a 103 after a 204. The baseline forwarded the
late header to net/http, which emitted an unstructured server diagnostic.
The wrapper now suppresses it and retains 204 on the wire and in the structured
request record. The same test still covers forwarding 103 before a final response.

## Follow-up

The Linux HTTP integration test also flushes a 204 response before hijacking the connection and requires the diagnostic to retain 204. A hijack before any final header continues to record 101 for the WebSocket path.

## Unsupported flush

`TestHTTPUnsupportedFlushLeavesStatusOpen` exercises a writer without flush support. A failed capability probe leaves the final status open, so the handler can still send a 503 and the operational record reports the same status.

The `TestHTTPUnsupportedFlushAllowsFallback` and
`TestHTTPUnsupportedHijackAllowsFallback` adapter regressions verify that
`http.ErrNotSupported` leaves diagnostics to classify the actual fallback
response. Errors from supported response I/O still produce failure records.
