# Linux signal with stranded readiness — 2026-11-15

## Scope

Bounded #18 continuation from merged PR #430, integration head `9dfb347`,
under accepted ADR-0068. GitHub confirmed #430 merged and the open PR queue
empty. #357 is closed. The human controls task delivery; the orchestrator is
inactive. The isolated task worktree preserves shared-checkout work.

Extend the existing `TestLifecycleSignals` harness with two cases: SIGINT
and SIGTERM while readiness never returns or observes cancellation. No
production code, contracts, dependencies or timeout changes.

## Evidence

**Integration-tested:** real signals reach Lifecycle/Lane in disposable Linux
children after the readiness callback writes its pipe checkpoint. Admission
stays closed; Start observes cancellation and joins; the later dependent stage
never starts. Stop receives a fresh context, waits for readiness until its
100 ms test budget expires, and returns the deadline error. The earlier stage's
Stop still receives that same expired deadline. Run reports deadline failure
and refuses reuse. The readiness callback remains stranded when the child
exits; successful readiness cancellation or graceful completion is not claimed.

Existing clean/error/panic and stuck-Stop cases remain controls: eighteen cases
total. The harness classifies failure as exit 23, rejects unexpected stdout or
stderr, and bounds/reaps each child within five seconds. These are fake runners
with the real Lifecycle and pinned Lane, without agents or paid APIs.

## Commands and outcomes

Local Linux amd64, Go 1.27.0; local results, not hosted CI. Go commands use
`GOCACHE=/tmp/amaru-after423-go-cache`.

- From `services/platform`, `go test -race -count=10 -run '^TestLifecycleSignals$' ./internal/platform`
  — PASS, exit 0; eighteen cases in ten repetitions (180 subprocess cases),
  no skips. Log: `/tmp/amaru-signal-readiness-race.log`.
- Repository root, `GOCACHE=/tmp/amaru-after423-go-cache make check`
  — PASS, exit 0, including Platform tests/vet, 348 TypeScript contract tests,
  346 matching Go/TypeScript fixture outcomes and 48 SDK tests.
  Log: `/tmp/amaru-signal-readiness-check.log`.
- `trail build` — PASS. `trail check --base 9dfb347` — exit 1, three lexical
  conflicts in the updated validation paragraph, reviewed individually:
  admission still closes before Start cancellation; no HTTP client changes or
  claim of fixing the historical speculative-connection race; no Session link
  closure or Worker ownership changes. Production code and those invariants
  remain unchanged. The TypeScript fixture advisory concerns existing tests,
  which passed. No learning is edited; this is not a passing Trail check.
- Final `GOCACHE=/tmp/amaru-after423-go-cache make docs-check` and
  `git diff --check` — PASS, exit 0 after the evidence update.

## Limits

No production entrypoint or exit wiring, owned-child reaping, actual PostgreSQL,
Providers, browser or native non-Linux run. No repeated-signal escalation,
pre-handler signal, uncooperative Start under signal, SIGKILL or process restart
evidence. No #18/M3 acceptance or new human architecture decision is claimed.
