# Project mutation authority after resource IO — Linux

Date: 2026-11-14. Runner: Codex for Ricardo Lüders. Local Linux x86_64,
kernel `7.2.11-100.fc43.x86_64`; physical/VM qualification not verified.
Tools: Go launcher 1.26.0, Platform toolchain 1.27.0, Node v25.7.0, pnpm 11.5.0.
Local evidence only, not CI or real Provider validation.

## Scope and review

Resumed existing worktree `/tmp/amaru-191-mutation-io`, branch
`fix/191-mutation-session-io`, at #18 integration base
`f732a09c7756f042b6843e570ab5c36d0ada1fa9`. Preserved untracked `.trail/cache`
and all existing worktrees. Live GitHub readback found #191/#18 open, no open
PRs, 25 open M3 issues and the designated base at that commit. No issue is closed by this slice.

Under accepted ADR-0062, `ProjectAccess.Cancel`, `DecideApproval`, `ResumeStep`
and `AbandonStep` now authorize again after successful resource ownership IO,
before calling the coordinator. Approval decisions check after both Approval and
gating Execution reads. The owner reviewed the initial identity-change test and
required failing closed when the authenticated human ID changes across the
checks. The final code refuses that case with `domain.ErrNotPermitted`, preserving
the original actor on successful calls. The second check never rebinds the actor.
Storage errors propagate first; missing/foreign ownership retains
`domain.ErrNotFound`, empty outcomes and no replay disclosure.

Architecture review: **sound** within the accepted boundary. Platform state and
trusted coordinator APIs remain authoritative; no Client, Worker, Provider,
domain or contract responsibility changes. No dependency, infrastructure,
prototype import, permission grant or new route. Checking only before the read
was insufficient because IO can block through session revocation. This refresh
is not atomic with the database commit: revocation after the check or during
coordinator IO is outside this fix. No persistence/transaction guarantee changes.
ADR-0070 Work commands remain proposed and untouched.

## Reproduction and validation

The new `TestProjectMutationAuthorityDuringIO` pauses a resource read using
channels, changes authority while it is blocked, then releases it without sleeps.
Five read stages (Cancel Execution, decision Approval, decision Execution, Resume
Execution and Abandon Execution) × fresh mutation/replay × seven outcomes yield
70 cases. Outcomes are authorized, revoked, cancelled, identity changed, storage
failure, missing and foreign. Error cases also revoke authority to verify error
precedence. Successful controls mutate and replay real coordinator commands
against fake storage. Denied cases assert empty results, false replay and no
additional coordinator command lookup or commit.

Before runtime edits, `go test -count=1 -run TestProjectMutationAuthorityDuringIO ./internal/coordinator`
failed with 20 revoked/cancelled mutation/replay cases (exit 1), log
`/tmp/191-mutation-before.log`. The initial test draft additionally exposed
monotonic/local time representation differences on JSON replay; using a fixed
UTC coordinator clock removed that test-only comparison problem before this
reproduction. The sandbox's first attempt could not write the existing Go cache;
the reproduction ran with access to that cache.

Final checks, all PASS, exit 0:

- From `services/platform`, `go test -race -count=100 -run '^TestProjectMutationAuthorityDuringIO$' ./internal/coordinator`:
  70 cases × 100 runs; log `/tmp/191-mutation-stress.log`.
- From `services/platform`, `go test -race -count=1 ./internal/coordinator ./internal/adapter/httpapi ./internal/adapter/localauth`:
  all three packages; log `/tmp/191-mutation-race.log`.
- Root `GOFLAGS=-p=1 make check`: all required checks passed, including
  348 TypeScript contract tests, 346 identical Go/TypeScript fixture outcomes,
  48 SDK tests, Platform/domain/architecture tests, Protobuf checks, host and
  cross-OS vet, PostgreSQL-tag vet and docs. Log `/tmp/191-mutation-check.log`.
- Final `make docs-check` and `git diff --check`: PASS, exit 0.

Cross-OS vet is a compatibility signal, not native runtime evidence.
PostgreSQL-tag vet is not PostgreSQL runtime validation.

Evidence level: **unit-tested** with fake authentication and storage, including
real coordinator mutation/replay logic. No actual logout/browser/HTTP route,
PostgreSQL runtime, product process, real Provider or native macOS/Windows
validation was performed for this slice. This is one bounded session-authority
regression; #191, #18 and M3 remain open with independent acceptance gates.
Existing actual-session settings evidence remains separate. No issue/Project
status or ADR decision changes are made.
