# Linux PostgreSQL HTTP forced drain — 2026-11-15

## Scope

Bounded #18 verification under accepted ADR-0068, from merged PR #425
(`a56398b`). Native prerequisite #15 is closed and no open PR was observed.
Work is isolated in `/tmp/amaru-next-425`; existing worktrees and shared
uncommitted changes are preserved. No production code, contract, dependency,
schema or architecture decision changes.

`TestPostgresHTTPWriteDrainsBeforeStorageCloses` now runs `ordinary` and
`forced` cases with the existing Storage/Schema/Recovery/HTTP composition,
authorized settings handler and actual PostgreSQL Project store.

## Evidence

**Integration-tested:** the ordinary case retains the prior successful drain
and durable commit assertions. In the forced case, a separate transaction locks
an existing settings row. The actual authorized PATCH reaches PostgreSQL;
`pg_stat_activity` and `pg_blocking_pids` establish that its upsert is blocked
before shutdown starts. No injected database or Provider behavior is used.

Shutdown closes admission while the pool and discovery remain available.
The unchanged five-second HTTP drain cap expires, cancels the request context
and forces the connection closed. The client cannot observe a successful write;
Lifecycle returns an error matching `context.DeadlineExceeded`. HTTP joins,
Auth clears its credential, discovery disappears and the owned pool closes.
An independent pool and fresh owners both observe the prior false setting.
Fresh owners rotate credentials, reject the old credential and stop normally.

The shared test cleanup budget is eight seconds to allow cancellation and joins
after the five-second HTTP cap. This tests cooperative SQL cancellation after
HTTP drain expiry, not exhaustion of the aggregate cleanup budget. It does not
establish atomic authorization/commit or reverse an already committed write.
Clients retain request-owned transports with keep-alives disabled, per the
existing HTTP lifecycle learning.

## Commands and outcomes

Local Linux 7.2.11-100.fc43.x86_64 amd64; Platform module Go 1.27.0; disposable PostgreSQL 17.5 on a private Unix socket,
TCP disabled. Go commands use `GOCACHE=/tmp/amaru-after423-go-cache`.
`AMARU_TEST_PG` points only at the disposable cluster. Results are local, not CI.

- From `services/platform`, `go test -race -count=1 -tags pg -run '^TestPostgresHTTPWriteDrainsBeforeStorageCloses$' -v ./internal/platform`
  — PASS, exit 0; ordinary and forced subtests, no skips.

- From `services/platform`, `go test -race -count=5 -tags pg -run '^TestPostgresHTTPWriteDrainsBeforeStorageCloses$' -v ./internal/platform`
  — PASS, exit 0; five repetitions of both cases, no skips.
  Log: `/tmp/amaru-next-425-repeat.log`.

- Repository-root `make check` — PASS, exit 0; docs, 348 TypeScript contract
  tests, 346 matching Go/TypeScript fixture outcomes, 48 SDK tests, default
  Platform tests and vet (including PostgreSQL-tagged/non-Linux compile checks).
  Log: `/tmp/amaru-next-425-check-final.log`.

- Final repository-root `make docs-check` and `git diff --check` — PASS, exit 0.

Initial attempts against a previously stopped disposable cluster failed because
the guessed roles did not exist; a fresh cluster with an explicit test role was
created. The initial sandboxed `make check` also stopped at pnpm store access;
the approved rerun used the existing store. A later docs-only attempt omitted
the writable Go cache and failed on the default cache path; rerunning with the
cache above passed. Staging required approved access to shared Git metadata. A sandboxed server start also failed to bind its Unix socket; approved
execution succeeded. These were environment setup failures, not test evidence.

## Review

`trail build` passed. `trail check --base a56398b` on the staged candidate exited 1
with three lexical documentation conflicts. The HTTP client ownership learning
is respected: request-owned transports still disable keep-alives. The test's
eight-second aggregate budget permits SQL cancellation and joining after the
existing five-second HTTP drain cap; it does not hide speculative connections
or change production timeouts. The session-link learning is unrelated to this
HTTP-only test change: no Session/Worker receive-pump teardown code changes.
Other lexical advisories do not introduce migration files, filesystem access in
storage, identity replacement or atomic authorization/commit guarantees, and
passing repetitions do not resolve earlier concurrency races. No learning is
edited and this is not a passing Trail semantic check.

## Limits

No production entrypoint/handler assembly, CLI, product Event transport,
messaging/outbox, bundled child ownership, process/server restart or kill,
real Provider, browser or native non-Linux validation. No M3 acceptance or
issue closure is claimed. No human architecture decision is needed.
