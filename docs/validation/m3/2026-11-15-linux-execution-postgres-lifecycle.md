# Linux PostgreSQL execution lifecycle composition — 2026-11-15

## Scope and prerequisites

Parent #18, based on merged PR #423 at
`30356b67813eb6ebf67ab0fbb3d6aec861a39214`. Native parent blocker #15 is
closed. This is a bounded verification slice of accepted ADR-0042/0044/0068,
with no new architecture decision, runtime behavior, dependency, migration,
contract or configuration. Work is isolated in `/tmp/amaru-after423`; the dirty
shared checkout and other worktrees are preserved. Current `main` (`d01934e`)
is merged for ancestry as required by the delivery workflow; its content was
already present, so that merge changed no files.

The previous PostgreSQL lifecycle test covered storage, migrations and recovery
of seeded running rows. Execution owner cancellation tests used fake storage.
The new opt-in `TestPostgresExecutionShutdownAndRecovery` exercises actual pgx,
PostgreSQL, Storage, Schema, Recovery, Session, EmbeddedWorker and Lifecycle
with a fake Runner and test-only permissive Policy. Ordinary and idempotent
Steps each use their own disposable database. Database creation/cleanup reuses
the existing storage test setup, extracted into `lifecycleDatabase`.

## Evidence

**Integration-tested:** a real Worker assignment reaches a durably running
Step. Process-context cancellation closes aggregate admission, cancels the
attempt and waits while its Runner deliberately holds cleanup. The owned pool
remains usable during that wait. After release, the lifecycle joins and closes
its pool. An independent observer connection verifies exact Execution, Step and
Event equality before/after shutdown, including the original lease/attempt:
no synthetic Result, explicit domain Cancel or terminal mutation is introduced.

Fresh owners on the same live server run migrations and reconciliation before
any execution stage. Ordinary running Steps become interrupted and their
Execution waits; idempotent running Steps requeue without a current lease.
Recovery adds durable Events. A third set of owners leaves those recovered
snapshots and Events unchanged before Worker startup. The idempotent case then
executes successfully with a new attempt and higher fence while retaining its
nonempty idempotency key. Every round verifies the owned pool closes and durable
state survives; the operator-owned server stays available.

This is component integration against a real database with a fake Runner.
Neither full process restart nor real Provider execution is claimed.

## Commands and outcomes

Linux 7.2.11-100.fc43.x86_64, amd64; Platform module Go 1.27.0;
Node 25.7.0; pnpm 11.5.0; disposable PostgreSQL 17.5. The cluster uses a
private Unix socket with TCP listening disabled. `AMARU_TEST_PG` points only
to that disposable cluster. Commands below are local results, not hosted CI.

- From `services/platform`,
  `go test -race -count=1 -tags pg -run '^TestPostgresExecutionShutdownAndRecovery$' -v ./internal/platform`
  — PASS, exit 0; ordinary and idempotent cases, no skips.
- From `services/platform`,
  `go test -race -count=20 -tags pg -run '^TestPostgres(ExecutionShutdownAndRecovery|StorageSchemaLifecycle)$' ./internal/platform`
  — PASS, exit 0; both tests repeated twenty times, no skips. Final log:
  `/tmp/amaru-after423-pg-race-final.log`.

- From the repository root, `make check` — PASS, exit 0: docs, 348
  TypeScript contract tests, 346 matching Go/TypeScript fixture outcomes, 48 SDK
  tests and all default Platform tests/vet. Log:
  `/tmp/amaru-after423-check.log`.

Go commands use `GOCACHE=/tmp/amaru-after423-go-cache`. Initial sandbox runs
could not write the default cache or bind/connect the Unix socket; reruns used
the task cache and approved socket access. Initial test development caught an
incorrect wait for Step success alone: the Coordinator's final Execution commit
can follow it. The test now waits for both terminal states. A subsequent compile
check caught a mistyped lease field, corrected before passing runs. Neither was
a demonstrated production defect.

## Review

`trail build` passed. `trail check --base 30356b67813eb6ebf67ab0fbb3d6aec861a39214`
exited 1 with eight lexical documentation conflicts, reviewed against the five
underlying learnings. These additions describe actual lifecycle integration:
SQL panic/stall handling is unchanged and no graceful broken-driver cleanup is
claimed; Lifecycle still closes admission before forwarding cancellation;
Session still joins its receive pump and retains links through Worker cleanup;
Project identity checks are unchanged; and no HTTP client or timeout is changed.
The two HTTP-test matches concern new PostgreSQL test descriptions, which use
no HTTP connections. The repeated PostgreSQL run is evidence for these bounded
assertions, not a claim that every historical race is resolved. No production
files or test-only runtime hooks are added. This is not a passing Trail check.

## Limits and remaining acceptance

No production entrypoint, process or PostgreSQL-server restart/kill, bundled
child ownership, messaging/outbox, HTTP/discovery/Event transport, real Provider,
complete diagnostic-channel capture or native non-Linux runtime is validated by
this test. It does not replace the existing crash-injection, timeout or security
suites. The permissive test Policy does not establish production authorization.
Default `make check` only compiles/vets PostgreSQL-tagged tests; their actual
runs are recorded separately above. #18/#20/M3 remain open. No human decision
is needed for this verification-only change.
