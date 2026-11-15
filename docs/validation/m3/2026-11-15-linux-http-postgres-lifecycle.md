# Linux PostgreSQL HTTP lifecycle composition — 2026-11-15

## Scope

Bounded #18 verification under accepted ADR-0068, based on merged PR #424
at `79c6a2a44e53d52d5455e4cc807512f1e1034485`. Native prerequisite #15 is
closed. No open PR was observed before selecting this task. Work is isolated in
`/tmp/amaru-after424`; shared checkout changes and other worktrees are preserved.
No production code, dependency, schema, contract or architecture decision changes.
Current `main` (`d01934e`) is merged for ancestry; its content was already
present and the merge changed no files.

Earlier tests composed HTTP/Auth/discovery with fake storage and composed
PostgreSQL with execution owners separately. The Linux/pg-tagged
`TestPostgresHTTPWriteDrainsBeforeStorageCloses` composes actual Storage,
Schema, Recovery and HTTP owners in successive Lifecycle stages, with the
existing authorized settings handler and PostgreSQL Project store. It reuses
`lifecycleDatabase`, `httpOwner`, `readyHTTP` and `httpCall`.

## Evidence

**Integration-tested:** initial authenticated HTTP GET observes default settings
after migrations/recovery. An admitted PATCH is held by a test wrapper before
the settings handler. Process-context cancellation closes aggregate admission:
another request receives 503, Lifecycle cannot return while the PATCH waits,
the owned database pool remains usable and discovery remains published.
Releasing the request allows its existing live authorization checks and atomic
settings update to finish successfully during ordinary HTTP drain.

After joins, Auth credentials are cleared, discovery is absent and the owned
pool rejects queries. An independent pool observes the committed value. Fresh
owners on the same server read it over authenticated HTTP, rotate the
administrator credential and reject the old credential with 401. Shutdown again
closes resources and retains settings. The test clients disable keep-alives per
the existing HTTP lifecycle ownership learning; no timeout is increased.

The blocked request is deliberately admitted before cancellation. This verifies
ordinary drain, not authorization revocation, forced timeout or atomic
authorization/commit. Existing security and timeout suites retain those scopes.

## Commands and outcomes

Local Linux 7.2.11-100.fc43.x86_64 amd64; Platform module Go 1.27.0;
Node 25.7.0; pnpm 11.5.0; disposable PostgreSQL 17.5, private Unix socket
with TCP disabled. `AMARU_TEST_PG` points only at that disposable server.
Go commands use `GOCACHE=/tmp/amaru-after423-go-cache`. These are local results,
not hosted CI.

- From `services/platform`,
  `go test -race -count=1 -tags pg -run '^TestPostgresHTTPWriteDrainsBeforeStorageCloses$' -v ./internal/platform`
  — PASS, exit 0; no skips.
- From `services/platform`,
  `go test -race -count=20 -tags pg -run '^TestPostgresHTTPWriteDrainsBeforeStorageCloses$' ./internal/platform`
  — PASS, exit 0; twenty repetitions, no skips.
  Log: `/tmp/amaru-after424-pg-repeat.log`.
- Final repository-root `make check` — PASS, exit 0; docs, 348 TypeScript
  contract tests, 346 matching Go/TypeScript fixture outcomes, 48 SDK tests,
  default Platform suites and vet (including PostgreSQL-tagged and non-Linux
  compile checks). Log: `/tmp/amaru-after424-check-final.log`.

The initial sandboxed `make check` stopped because pnpm could not open its store
database. An approved rerun then caught documentation links added before this
record existed. The record was added before the final checks; neither failure
was a runtime defect. A preliminary compile run without `AMARU_TEST_PG` skipped
the test and is not counted as runtime evidence.

## Review

`trail build` passed. `trail check --base 79c6a2a44e53d52d5455e4cc807512f1e1034485`
exited 1 with seven lexical documentation conflicts, reviewed against the five
underlying learnings: HTTP clients retain request-owned transports with disabled
keep-alives; Lifecycle still closes admission before forwarding cancellation;
SQL panic containment is unchanged, with no broken-driver cleanup claim;
Coordinator receive-pump joins remain unchanged; and Project identity checks
are unchanged, with no atomic authorization/commit claim. Repeated tests validate
only the explicit assertions here; they do not disprove historical races.
No existing learning is edited. This is not a passing Trail check.

## Limits

No production entrypoint, canonical directory selection, production handler
assembly, CLI, product Event/WebSocket protocol, messaging/outbox, bundled
database ownership, process/server restart/kill, real Provider, browser or native
non-Linux behavior is validated here. PostgreSQL-tagged tests are compiled/vetted
by Linux `make check`, but run separately as recorded above. This does not finish
#18, #92, #191 or #20/M3 acceptance. No human decision is needed for this
verification-only change.
