# Linux Lifecycle signals during startup — 2026-11-15

## Scope

Bounded #18 continuation from merged PR #428 (`47cc144`), under accepted
ADR-0068. Extend the existing Linux-only `TestLifecycleSignals` subprocess
harness to deliver SIGINT and SIGTERM while a runner blocks in `WaitReady`.
No production code, signal owner, timeout, contract or dependency changes.
Shared checkout changes are preserved in isolated `/tmp/amaru-next-task`.

## Evidence

**Integration-tested:** actual Lifecycle and pinned Lane v2.1.0 receive OS
signals in an isolated child. A fixed pipe marker synchronizes signal delivery
with the blocked startup readiness callback; the parent uses no timing delay
to choose this checkpoint. The readiness callback returns only after context
cancellation. Stop checks closed admission, a fresh cleanup context and joins
both Start and readiness before completing or returning the injected failure.
The earlier stage still stops in reverse order, including after Stop error or
panic. The later dependent stage must never start in startup cases; ready
cases provide a positive control that it starts once readiness completes.

The existing test-owned exit classification and silent stdout/stderr assertions
remain. Both signals exercise clean cleanup, Stop error and Stop panic in both
phases: twelve subprocess cases total, six added. Tests use fake runners and
need no real agents, paid APIs or server. This is helper integration evidence,
not a production process startup/shutdown claim.

## Commands and outcomes

Local Linux 7.2.11-100.fc43.x86_64 amd64, module Go 1.27.0. Local evidence, not CI.
Go commands below use `GOCACHE=/tmp/amaru-after423-go-cache` unless stated.

- From `services/platform`, `go test -race -count=1 -run '^TestLifecycleSignals$' -v ./internal/platform`
  — PASS, exit 0; twelve cases, no skips (default cache on this initial run).

- From `services/platform`, `go test -race -count=10 -run '^TestLifecycleSignals$' ./internal/platform`
  — PASS, exit 0; all twelve cases in ten repetitions, no skips.
  Log: `/tmp/amaru-next-task-signals-race.log`.

- Repository root, `GOCACHE=/tmp/amaru-after423-go-cache make check`
  — PASS, exit 0, with approved access to the existing pnpm store and loopback
  sockets; 346 identical Go/TypeScript fixture outcomes and 48 SDK tests.
  Log: `/tmp/amaru-next-task-check-approved.log`.
- `trail build` — PASS. `trail check --base 47cc144` — exit 1, three
  reviewed lexical documentation conflicts. The admission assertion preserves
  closed admission before Start cancellation; no parent-context forwarding is
  changed. No Session/Worker link ownership or cleanup is changed. No HTTP
  client or timeout changes occur; these signal repetitions do not resolve
  the historical speculative-connection race. Other advisories change no
  storage, authorization, OS scope or process-output handling. Learnings remain
  unchanged.

The first repetition/full-check attempts could not write the default Go cache;
using the writable cache above resolved that setup failure. An interim docs
check caught the new evidence link before this file was created; it was
corrected. The sandboxed full check then failed opening the existing pnpm
store database. These attempts are not passing validation.

## Limits

No production entrypoint/exit wiring, real PostgreSQL or Providers, owned-child
reaping, repeated-signal escalation, signals before Lane registers handlers,
SIGKILL recovery, server restart, browser or native non-Linux validation.
Startup coverage blocks cooperative `WaitReady`; it does not establish cleanup
of an uncooperative readiness callback. Existing aggregate-budget tests cover
timeout separately. No #18/M3 completion or human architecture decision.
