# Linux Lifecycle signal handling — 2026-11-15

## Scope

Bounded #18 continuation from merged PR #427 (`1f49e7d`), under accepted
ADR-0068. `TestLifecycleSignals` adds Linux-only subprocess integration
coverage. No production code, signal owner, timeout, contract or dependency
changes. Shared work is preserved in isolated `/tmp/amaru-after427`.

## Evidence

**Integration-tested:** the parent sends actual SIGINT or SIGTERM only after
the child reports aggregate readiness over its stdout pipe. Actual Lifecycle
and pinned Lane v2.1.0 own signal handling; no test signal handler or caller
cancellation substitutes for it. Fake dependent runners assert that admission
closes before Start observes cancellation, Stop receives a fresh context and
joins Start, and both stages stop in reverse order. A Stop error or panic does
not suppress the earlier stage's cleanup. Underlying error/panic classification
is retained without printing unsafe values.

The test-owned harness exits 0 after clean cleanup or 23 after classified Stop
failure. The parent checks exit status, rejects unexpected stdout/stderr and
bounds the child lifetime to five seconds. Only a fixed readiness marker is
allowed on stdout; Lane diagnostics use the existing nil bounded handoff.
Six cases run in default Linux `make check`, without helper skips.

This is process-signal evidence for the Lifecycle composition helper and fake
owners, not production bootstrap or execution cancellation semantics.

## Commands and outcomes

Local Linux 7.2.11-100.fc43.x86_64 amd64, Go 1.27.0. Local results, not CI.
Go commands use `GOCACHE=/tmp/amaru-after423-go-cache`.

- From `services/platform`, `go test -race -count=1 -run '^TestLifecycleSignals$' -v ./internal/platform`
  — PASS, exit 0; six signal/cleanup cases, no skips.

- From `services/platform`, `go test -race -count=10 -run '^TestLifecycleSignals$' ./internal/platform`
  — PASS, exit 0; all six cases in ten repetitions, no skips.
  Log: `/tmp/amaru-after427-signals-race.log`.
- Repository root, `GOCACHE=/tmp/amaru-after423-go-cache make check`
  — PASS, exit 0; log `/tmp/amaru-after427-check.log`.
- From `services/platform`, `go test -race -count=1 ./internal/platform`
  — PASS, exit 0, with approved loopback access; log
  `/tmp/amaru-after427-owner-race-approved.log`.
- Final repository-root `GOCACHE=/tmp/amaru-after423-go-cache make docs-check`
  and `git diff --check` — PASS, exit 0.
- `git diff --cached --check` — PASS, exit 0.
- `trail build` — PASS. `trail check --base 1f49e7d` — exit 1, five reviewed
  lexical documentation conflicts. The test asserts closed admission before
  Start observes cancellation and joins Start from Stop; it does not change
  cancellation forwarding, Session/Worker link ownership or receive pumps.
  HTTP clients and cleanup timeouts are unchanged. Other advisories introduce
  no storage filesystem imports, identity, schema or contract changes; passing
  repetitions do not resolve historical concurrency races. Learnings remain
  unchanged.

An initial gofmt command used a repository-root path from the module directory
and failed to find the file. The corrected module-relative command succeeded;
the test command in that initial shell invocation independently passed.
The first complete owner race suite was denied loopback sockets by the sandbox;
that failure is environmental, not passing evidence. An approved rerun permits
loopback access. The full `make check` also used approved access to the existing
pnpm store; staging needed approved access to shared worktree Git metadata.

## Limits

No production entrypoint/exit wiring, owned-child reaping, real PostgreSQL or
Provider, HTTP/Event transport, signal during startup, repeated-signal
escalation, SIGKILL recovery, server restart, browser or native non-Linux
validation. Stop timeout is covered separately by the existing aggregate-budget
test; signal handling does not establish graceful cleanup after a timeout.
No #18/M3 completion, issue closure or human architecture decision.
