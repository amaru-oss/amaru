# PRJ-002: PostgreSQL settings write after session logout

## Selection and sync

On 2026-11-15, PR #417 was verified merged into #18 at
`2e44b7085cd99d24b846382e51423d2f68daf4af`. The isolated checkout was synced to
that commit; merged-result `make check` passed (exit 0), log
`/tmp/amaru-417-merged-check.log`. Shared user work and other worktrees were
preserved. No open PRs were reported. #191's native #120 and inherited #15
blockers remain closed.

Selected the missing PostgreSQL evidence for #417's committed-write/denied-response
behavior. This change reuses the existing session regression against fake and
PostgreSQL storage, with a fresh migrated disposable database per run and pool
recreation readback. No production code, dependency, schema, contract or ADR
changes. Full #191/#18/M3 acceptance remains open.

## Integration-tested

The shared regression uses actual launch/session/CSRF/logout HTTP adapters with
`httptest`. A trusted store callback logs out the session after the settings
write returns from the real PostgreSQL store (after commit), before returning
to the application. The PATCH returns redacted 403 with no settings; subsequent
requests from that session return 401. A separate session reads the committed
true value and the other Project's unchanged false default. Closing the whole
database pool and creating a new pool/store preserves both values.

With pre-#417 `project_settings.go` from `990aed5`, the PostgreSQL regression
failed: HTTP 200 and saved settings instead of 403. The merged source was
restored in a `finally` block before final checks. Log:
`/tmp/amaru-418-pg/settings-before.log`.

## Local commands and results

Linux amd64, PostgreSQL 17.5 from the existing cached distribution, installed
Go 1.26.0 (module selects Go 1.27), Node v25.7.0 and pnpm 11.5.0.
The owned disposable PostgreSQL server uses only a Unix socket beneath a 0700
directory, with no TCP listener. `AMARU_TEST_PG` selects that private socket;
tests create/drop their own databases. No existing development data is used.

From `services/platform`, with that environment:

- `go test -count=1 -tags pg -run '^TestSettingsPostgresSessionLogoutDuringWrite$' -v ./internal/adapter/httpapi`
  — FAIL against pre-#417 source, as described above.
- `go test -race -count=20 -tags pg -run '^TestSettings(PostgresSessionLogoutDuringWrite|SessionLogoutDuringWrite)$' -v ./internal/adapter/httpapi`
  — PASS, exit 0, 40 runs with no skips. Log:
  `/tmp/amaru-418-pg/settings-race.log`.
- `go test -race -count=1 -tags pg -run TestSettings -v ./internal/adapter/httpapi`
  — PASS, exit 0, 14 top-level tests, no skips. Log:
  `/tmp/amaru-418-pg/all-settings-race.log`.

From the repository root:

- `make check` — PASS, exit 0, including 348 TypeScript contract tests,
  346 identical fixture outcomes and 48 SDK tests. Log:
  `/tmp/amaru-418-pg/make-check.log`.
- Final `make docs-check`, `git diff --check` and `git diff --cached --check`
  — PASS, exit 0.
- `trail build` — PASS. `trail check --base 2e44b7085cd99d24b846382e51423d2f68daf4af`
  — exit 1, one lexical conflict in the canonical documentation against the
  storage-filesystem-boundary learning. No storage or filesystem imports change;
  this adds integration evidence using existing adapters. Advisories match
  unchanged clone/migration, Workflow, lifecycle, cancellation and scheduling
  rules. No passing automated Trail review is claimed.

The owned server was stopped successfully with `pg_ctl stop -m fast`; logs and
the isolated validation cluster remain under `/tmp/amaru-418-pg`.

## Limits

This verifies HTTP/auth/application/PostgreSQL integration, not browser execution,
product bootstrap, real Providers, native non-Linux behavior or database/process
restart. The Coordinator is unused and fake-backed in this settings-only path.
Authorization is not atomic with commit or response delivery; denial does not
imply rollback. These local results are not hosted CI. Proposed ADR-0070/0071
are untouched; no issue closure or merge is claimed.
