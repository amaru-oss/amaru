# Linux PostgreSQL minimum-version startup gate — 2026-11-15

## Scope and prerequisite

Parent #18, after merged PR #422 at `e29d747629f927f7fea7168e23afb027ee0d67ed`.
The isolated working branch also merges current `main` (`d01934e`); those planning
changes were already present as content from the squashed child PR. The merge
restores ancestry without additional file changes. Shared checkout work was
preserved.

Accepted ADR-0015 and ADR-0045 require PostgreSQL 16 or newer. ADR-0068 places
storage completion before migrations and dependent admission. Previously Storage
only pinged the configured connector; it admitted a fake PostgreSQL 15 server.
No new architecture choice, dependency, migration or public setting is introduced.

## Change and evidence levels

**Unit-tested with real Lane:** after connectivity, Storage executes
`SHOW server_version_num` and scans its result into an integer under the existing
absolute startup deadline. Versions below 160000 return the fixed error
`storage requires PostgreSQL 16 or newer`. Query/scan failure withholds the pool
and readiness. Existing normal rollback closes the pool; the operator-owned
server is never stopped. Tests cover 15 rejection, 16/17/18 acceptance, malformed
and null results, no row, driver error, cancellation and deadline expiry. The
shared recovery fake forwards the version query to the storage fake, retaining
its independent reconciliation failure hooks.

The [PostgreSQL preset-options reference](https://www.postgresql.org/docs/18/runtime-config-preset.html)
defines `server_version_num` as the integer server version. Numeric comparison
avoids parsing human-readable release labels. Raw query/scan errors remain
unsafe to print, as at the existing Storage boundary; safe lifecycle logging
continues to omit arbitrary returned error content.

**Integration-tested:** existing `TestPostgresStorageSchemaLifecycle` ran through
actual pgx, PostgreSQL 17.5, migrations, recovery, downstream readiness, rollback
and owner recreation. A fresh disposable cluster used a private Unix socket and
no TCP listener; it was stopped afterward. This validates the supported-server
path, not native PostgreSQL 15/16/18 or a production entrypoint.

## Commands and outcomes

Environment: Linux 7.2.11-100.fc43.x86_64, amd64; module Go 1.27.0; Node 25.7.0;
pnpm 11.5.0; disposable PostgreSQL 17.5.

- Before the fix, from `services/platform`:
  `go test -count=1 -run '^TestStoragePostgresVersionGate$' ./internal/platform`
  — FAIL, exit 1: old, malformed and null versions admitted downstream work.
- `go test -race -count=1 -tags pg -run '^TestPostgresStorageSchemaLifecycle$' -v ./internal/platform`
  with `AMARU_TEST_PG` set to the disposable server — PASS, exit 0, no skips.
  Log: `/tmp/amaru-after422-pg-test.log`.
- `go test -race -count=1 -run '^TestStoragePostgresVersionGate$' ./internal/platform`
  — PASS, exit 0.
- `go test -race -count=20 -run '^Test(Storage|Schema|Recovery|DriverPanic|LifecycleStartupRollback)' ./internal/platform`
  — PASS, exit 0 (58.029s), no skips. Log:
  `/tmp/amaru-after422-storage-race.log`.
- From the repository root, `make check` — PASS, exit 0, including docs-check,
  348 TypeScript contract tests, 346 matching Go/TypeScript fixture outcomes,
  48 SDK tests and all default Platform tests/vet. PostgreSQL-tagged and
  non-Linux vet are compilation checks, not runtime evidence. Log:
  `/tmp/amaru-after422-check.log`.

- Final `make docs-check`, `git diff --check` and staged whitespace check — PASS, exit 0.

## Review

The change stays in the existing concrete Storage owner, before Schema and
Recovery. No domain, port, contract or Worker change is required. A bare ping
cannot establish the already approved server-version requirement. No human
architecture decision is pending for this implementation.

Trail's cancellation-ordering match in the storage test is lexical: these are
existing-style resource-owner tests using Lane directly, with no public readiness
latch. Production `Lifecycle` and its `WithoutCancel`/close-before-forward ordering
are unchanged. Driver panic limitations remain explicit; no retry or migration
compatibility path is introduced.

## Limits and remaining acceptance

No native PostgreSQL 15/16/18 runtime, non-Linux runtime, real Provider,
bundled PostgreSQL child ownership, messaging/outbox, full product startup or
complete process-channel capture was validated. Fake query cancellation is
cooperative; driver panic/stall protections and their process-exit limitations
remain as documented in the storage lifecycle evidence. This check does not
validate every future pool connection or protect against a server replacement
after startup. #18/#170/#191/#219 and M3 acceptance remain open. Local checks
are not hosted CI.
