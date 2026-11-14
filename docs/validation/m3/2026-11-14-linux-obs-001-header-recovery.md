# OBS-001: HTTP status after a recovered header panic

## Selection and queue

Local Linux evidence on 2026-11-14, starting from merged #18 integration head
`4339e5666f74b090fae1d196303718b8f3ff0200` (PR #414). GitHub was refreshed
before source inspection; the remote integration branch was exactly this head.
No PRs were open. Recent #409–#414 were merged into
`feat/18-wp-m3-06-public-api-events`.

- #18 is open. Its native blocker #15 is closed.
- #170 (OBS-001) is open. Direct native blockers #83/#84 are closed, as is
  inherited #15. No catalog dependency is listed.
- Native children #92/#191/#219 remain open; #83/#84/#106/#120/#135/#335/#339/#357
  are closed. #191's direct blocker #120 and #219's direct blocker #135 are closed.
- Proposed ADR-0070/0071 remain human gates. ADR-0071 is absent from this base;
  no acceptance is inferred. This change implements neither proposed contract.

Selected #170 because a real, bounded gap reproduced: `response.WriteHeader`
updated its diagnostic status and final-header flag before the underlying
writer validated the code. If a handler recovered that panic, subsequent valid
headers were suppressed. A requested 204 became wire 200, while diagnostics
claimed 99 or 1000 (including a false `rejected` outcome).

Accepted [ADR-0059](../../architecture/adr/0059-adapt-the-prototype-http-handlers-to-the-platform-public-api.md)
covers the HTTP/secret-safe diagnostics boundary. The fix records final status
only after `WriteHeader` returns. No new component, public contract, dependency,
infrastructure or prototype import is needed.

## Scope and evidence

**Integration-tested:** `TestHTTPRecoveredHeaderPanicStatus` uses real loopback
HTTP with invalid codes 99/1000, handler recovery and final explicit 204,
implicit body-write 200 or flush 200. All six cases assert matching wire/JSON
status, successful outcome, correlation identity, no request/body fixtures in
JSON and no raw server diagnostics. All six failed before the fix.

The shared wrapper's callers are the Platform HTTP owner, SDK auth fixture and
operational tests. The response methods `Write` and `FlushError` also call
`WriteHeader`. Call sites were searched before editing; no caller, config,
export or contract fixture needs changing. Existing informational/101 behavior,
I/O diagnostics, auth rejection and panic containment remain covered by the
focused suite. The implementation changes only the final-header assignment
order and adds a regression in `http_test.go`.

## Checks

Environment: Linux 7.2.11-100.fc43.x86_64, Go 1.26.0 linux/amd64,
Node v25.7.0, pnpm 11.5.0. Local results, not hosted CI.

From `services/platform`:

- Before fix: `go test -count=1 -run '^TestHTTPRecoveredHeaderPanicStatus$' ./internal/adapter/operational`
  — FAIL, all six cases, reproducing wire/diagnostic mismatch.
- `TMPDIR=/home/rluders/amaru-m3-170-tmp go test -race -count=20 -run '^TestHTTP' ./internal/adapter/operational`
  — PASS, exit 0; `header-panic-http-race.log`.
- `TMPDIR=/home/rluders/amaru-m3-170-tmp go test -race -count=1 -run '^TestHTTP' ./internal/platform`
  — PASS, exit 0; `header-panic-composition-race.log`.

From repository root:

- `TMPDIR=/home/rluders/amaru-m3-170-tmp make check` — PASS, exit 0;
  `header-panic-make-check.log`. Includes 348 TypeScript contract tests,
  346 identical Go/TypeScript fixture outcomes and 48 SDK tests, plus Platform
  tests and cross-OS vet (not native execution).
- `TMPDIR=/home/rluders/amaru-m3-170-tmp make docs-check` — PASS, exit 0;
  `header-panic-docs-check.log` (`doccheck: ok`).
- `git diff --check` and `git diff --cached --check` — PASS, exit 0.
- `trail check --base 4339e5666f74b090fae1d196303718b8f3ff0200` — PASS,
  exit 0; `header-panic-trail-check.log`, no conflicts. The pre-stage run had one
  advisory; the final four-file run had ten lexical advisories, including this
  evidence file. All were reviewed: they concern unrelated storage, Git source,
  Project-board, concurrency or lifecycle code, or the temp-directory trap
  explicitly avoided here. No conflicting approach is reintroduced.

All named logs are retained in `/home/rluders/amaru-m3-170-tmp`, outside every
checkout. An initial post-fix focused attempt was blocked by sandbox access to
the Go build cache; it is not passing evidence. The subsequently interrupted
validation tool call supplies no result. The completed runs above used the
explicit external TMPDIR requested by the owner.

## Trail review and limits

Read `graft ask` context before source inspection, followed by Trail's HTTP
context and `graft ask ... --source`. Ricardo's temp-directory learning prevented
the documented Git-root discovery trap. Reviewed the suggested HTTP lifecycle
connection-ownership warning; this regression owns a server/client per case and
closes the server before reading captured buffers. No existing worktree was
modified or removed; pre-existing `.trail/cache/` remains untracked.

The status means the underlying header call returned; it does not prove network
delivery or recover a partial response. A handler still owns its recovery policy.
No production bootstrap, real Provider, PostgreSQL runtime, browser or native
macOS/Windows run is claimed. Full #170, #18 and M3 acceptance remain open. The
PR is for orchestrator review only; no merge, issue closure or worktree cleanup.
