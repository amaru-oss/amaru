# Linux signal shutdown budget — 2026-11-15

## Scope

Bounded #18 continuation from merged PR #429, integration head `d653cee`,
under accepted ADR-0068. Native parent prerequisite #15 and lifecycle decision
#357 were verified closed; the open PR queue was empty. Reuse the isolated
`/tmp/amaru-next-task` worktree and preserve the shared checkout's changes.

Extend `TestLifecycleSignals` with a Stop callback that ignores cancellation
and never returns. Both SIGINT and SIGTERM exercise this failure after aggregate
readiness and while startup readiness is blocked: four new cases, sixteen
subprocess cases in total. No production code, contract, dependency, signal
ownership or product timeout changes.

## Evidence

**Integration-tested:** real OS signals reach the existing Lifecycle and pinned
Lane v2.1.0 in disposable child processes. A pipe marker synchronizes signal
delivery with readiness. The stuck Stop first verifies closed admission,
a fresh cleanup context and joined Start/readiness callbacks. It then blocks
forever with a test-only 100 ms shutdown budget.

Run must return a deadline failure. The earlier stage's Stop must still be
invoked with the same, already-expired deadline, without renewing its budget.
The test waits explicitly for that observation: late Stops can run after Run
returns when the cleanup budget has expired. The earlier Stop's invocation
does not imply that the stuck Stop completed. Reusing the Lifecycle must fail.
The child maps the failure to exit 23, with no unexpected stdout/stderr; the
parent's five-second deadline bounds the whole child and always reaps it.
The child exits with its stranded goroutine; graceful cleanup is not claimed.

Existing clean/error/panic cases remain positive controls. Startup cases must
never start the later dependent stage; ready cases must start it. The runners
are fakes; no agents, paid APIs or external server are needed.

## Commands and outcomes

Local Linux amd64; module Go 1.27.0. These are local results, not CI.
Go commands use `GOCACHE=/tmp/amaru-after423-go-cache`.

- From `services/platform`, `go test -race -count=1 -run '^TestLifecycleSignals$' -v ./internal/platform`
  — PASS, exit 0; sixteen cases, no skips, initial candidate. The final candidate
  additionally waits for the late Stop observation described above.
- From `services/platform`, `go test -race -count=10 -run '^TestLifecycleSignals$' ./internal/platform`
  — PASS, exit 0; sixteen cases in ten repetitions (160 subprocess cases), no
  skips. Log: `/tmp/amaru-signal-budget-race.log`.
- Repository root, `GOCACHE=/tmp/amaru-after423-go-cache make check`
  — PASS, exit 0, with approved pnpm cache/loopback access; 348 TypeScript
  contract tests, 346 matching Go/TypeScript fixture outcomes and 48 SDK tests.
  Log: `/tmp/amaru-signal-budget-check.log`.
- `trail build` — PASS. `trail check --base d653cee` — exit 1, three reviewed
  lexical documentation conflicts. Admission still closes before Start
  cancellation; no parent-context forwarding or Session/Worker link ownership
  changes. No HTTP client changes or claim of fixing the historical speculative
  connection race. The cleanup-budget advisory changes only this test's stuck
  Stop timeout to 100 ms, without changing any product/HTTP budget. The fixture
  advisory concerns existing TypeScript tests, which passed here. Learnings
  remain unchanged; this is not a passing Trail semantic check.

## Limits

No production entrypoint/exit wiring, owned-child reaping, real PostgreSQL or
Providers, repeated-signal escalation, signal before handler registration,
SIGKILL/restart recovery, browser or native non-Linux validation. The stranded
callback is Stop; Start and readiness remain cooperative. Existing watchdog
tests cover uncooperative startup separately. No #18/M3 completion, new
infrastructure or human architecture decision is claimed.
