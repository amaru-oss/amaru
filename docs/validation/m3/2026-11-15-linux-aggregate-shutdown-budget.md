# Linux aggregate shutdown budget — 2026-11-15

## Scope and evidence

Bounded #18 continuation after PR #426, integration base `217f78d`.
Accepted ADR-0068 already requires one shared cleanup budget, failure on
exhaustion and no in-process restart. No production behavior, timeout,
contract, dependency or architecture decision changes. Shared checkout work
is preserved in an isolated `/tmp/amaru-after426` worktree.

**Integration-tested:** `TestLifecycleAggregateShutdownBudget` runs actual
Lifecycle/Lane with fake dependent runners in disposable child processes.
After healthy readiness, caller cancellation strands either Start or Stop.
The first Stop receives a fresh cleanup context after admission closes.
Run returns `context.DeadlineExceeded` within a one-second tolerance for a
100 ms test budget. Both earlier-stage Stops are invoked with the identical
deadline, already expired at invocation. Admission stays closed and a second
Run is rejected. The harness exits with code 23 and no output while a callback
remains stranded; the parent rejects hangs, unexpected output and other exits.

This demonstrates test-owned failure classification and subprocess isolation,
not a production entrypoint. It makes no graceful-cleanup claim for abandoned
callbacks. No external services or paid APIs are needed; both scenarios run
in `make check` without helper skips.

## Commands and outcomes

Local Linux 7.2.11-100.fc43.x86_64 amd64, Go 1.27.0. Local results, not CI.

- From `services/platform`, `go test -race -count=20 -run '^TestLifecycleAggregateShutdownBudget$' ./internal/platform`
  — PASS, exit 0; both scenarios in all twenty repetitions.

- From `services/platform`, `go test -race -count=1 ./internal/platform`
  — PASS, exit 0; complete owner suite.
- Repository root, `GOCACHE=/tmp/amaru-after423-go-cache make check`
  — PASS, exit 0; log `/tmp/amaru-after426-check.log`.
- `git diff --cached --check` — PASS, exit 0.
- `trail build` — PASS; `trail check --base 217f78d` — exit 1, four
  reviewed lexical documentation conflicts. The cancellation-order learning
  remains respected: Lifecycle still closes admission before forwarding parent
  cancellation, and the test asserts the closed latch at Stop. The HTTP-client
  learning concerns speculative connections; this test creates no HTTP clients
  and changes no HTTP budget. Other advisories introduce no SQL driver, receive
  pump, identity, concurrency-limit or contract change. Repetitions do not resolve
  unrelated historical races. No learning was edited.

The first sandboxed `make check` stopped at the read-only default Go cache;
using the writable cache reached pnpm and stopped at store access. The approved
rerun passed with the existing dependency store. Initial staging also needed
approved access to the shared worktree Git metadata. These setup failures are
not passing validation evidence.

## Limits

No production bootstrap/exit wiring, owned child reaping, real database or
Provider, HTTP/Event transport, process/server restart or kill, browser or
native non-Linux validation. No #18 or M3 completion or issue closure.
No human architecture decision is required.
