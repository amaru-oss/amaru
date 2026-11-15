# Linux lifecycle deadline diagnostics — 2026-11-15

## Scope and baseline

Bounded OBS-001 [#170](https://github.com/rluders/amaru/issues/170) continuation
from merged PR #421, #18 integration commit `b3a0784`. Native #170 prerequisites
#83/#84 and inherited #18 prerequisite #15 were verified closed. No other PR
targeted the integration branch at selection. Shared checkout changes were
preserved by using `/tmp/amaru-after421`.

`Lifecycle.Run` already fails on startup/caller deadline expiry. Before this
change, expiry before startup emitted no diagnostics; Lane treated watchdog
cancellation as intentional and did not emit an error diagnostic. The focused
regression failed for expired-startup, active-startup and caller-deadline cases
on the unchanged implementation (exit 1); intentional cancellation passed.

The lifecycle boundary now tries one fixed `process.lifecycle` error record on
deadline failure before returning, using `startup_deadline_exceeded` or
`caller_deadline_exceeded`. Startup expiry takes precedence. It neither formats
the returned error nor reads a custom context cause. Existing returned errors,
admission ordering, cleanup budgets and Lane's severity-only records are unchanged.

## Evidence

`TestLifecycleDeadlineDiagnostics` uses actual Lane with cooperative fake
resource owners, a buffered diagnostic queue and `Logger.ConsumeLifecycle`.
It parses every JSON record, checks trusted Instance/correlation identity,
requires one fixed deadline error record and proves an unregistered custom
context-cause fixture is absent. Intentional cancellation adds no error record.
`TestLifecycleDeadlineDiagnosticDoesNotWaitForConsumer` proves nil, unbuffered
and full queues cannot block the deadline return; a full queue keeps its sentinel.

Local Linux checks (not hosted CI):

- Before the fix, `go test -count=1 -run '^TestLifecycleDeadlineDiagnostics$' ./internal/platform`: FAIL, exit 1, all three deadline cases reproduced the missing diagnostic.
- `go test -race -count=20 -run '^TestLifecycle(DeadlineDiagnostics|DiagnosticComposition|FailuresAndIntentionalCancellation|WatchdogBoundsUncooperativeStartup)$' ./internal/platform`: PASS, exit 0.
- `go test -race -count=20 -run '^TestLifecycle' ./internal/platform`: PASS, exit 0, including the final nil/unbuffered/full queue regression; no skips.
- `make check`: PASS, exit 0; 348 TypeScript contract tests, 346 matching Go/TypeScript fixture outcomes, 48 SDK tests and the default Platform suites. PostgreSQL-tagged and non-Linux code is vetted, not runtime-tested.
- `git diff --check`: PASS, exit 0.
- `trail check --base b3a0784`: exit 1, one lexical README conflict with Ricardo's admission-before-cancellation learning. This change only adds deadline diagnostics after teardown; `context.WithoutCancel`, the readiness latch and ordered forwarding of cancellation are unchanged. The passing admission/lifetime/rollback race tests cover those invariants. Other matches are advisory, not failures.

Go commands run from `services/platform`; Make commands from the repository root.
The first full check stopped on the evidence link before this file was created;
it was not a code-test failure. Final validation must include the complete file.

Environment: Linux `7.2.11-100.fc43.x86_64`, amd64; module-selected Go 1.27.0;
Node 25.7.0; pnpm 11.5.0. Local logs:
`/tmp/amaru-after421-race.log` and `/tmp/amaru-after421-check.log`.

## Limits

This is **integration-tested** lifecycle/diagnostic composition with fake owners,
not production startup or a real Provider run. Queues remain best effort: nil,
unavailable or full queues drop records. Lane may emit other error diagnostics
for concurrent cleanup failures; the fixed cause identifies deadline failure,
not every underlying failure. A stalled sink still requires a bounded consumer
join and unsuccessful process exit in the future production entrypoint.

No PostgreSQL runtime, full product stdout/stderr capture, real Provider or native
macOS/Windows runtime validation was performed. Existing process-failure and
child-ownership obligations remain. #170, #18 and M3 remain open; no architecture
decision, dependency, public contract or production entrypoint was introduced.
