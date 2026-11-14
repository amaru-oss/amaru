# ART-002 Artifact capture replay conflicts — Linux

Date: 2026-11-14. Local Linux x86_64, `7.2.11-100.fc43.x86_64`.
Local results are not hosted CI or real Provider validation.
Branch: `fix/124-artifact-replay-conflicts`; isolated worktree
`/tmp/amaru-124-artifact-replay`. Base: #19 integration
`feat/19-wp-m3-07-worktrees-configuration-scopes` at
`1e5c9e6d93fa1e0acdc1c0b0c99d84e3d5f8320e` (merged #397).
Source: this record's commit. Existing worktrees and untracked data were preserved.

## Selection and live queue

Authenticated GitHub REST/gh inspection verified #398 at the requested head
`7dc411da90ebb504498c763596e415aa8ded6073`, targeting #18 at `2ea3069`.
Independent review found no actionable code issue in the child-process assertions,
Runner observation, Worker panic/cancellation or Session/Lifecycle joins.
The account `rluders` had repository admin/push permission but was also PR author.
Formal approval returned HTTP 422, `Review Can not approve your own pull request`.
An exact-head COMMENT review recorded findings and validation; merge succeeded
at `39386f7e9cd2782075d1cf865955a39ccef71a26`. Neither #170 nor #18 was closed.
PR #398's historical prose misidentifies #397 as a check-result assessment;
live #397 is the ambiguous Step identity fix. #384 Usage, #394 walkthrough,
#397 Step identities and #398 channel capture are merged and not duplicated.

Native GitHub prerequisites were read, including inherited parents: #124's
#119/#123 and #19's #15 are closed; required code is on this base.
#170/#92 have closed #83/#84 and inherited #15; #191 and #154 have closed
#120; #154 also has closed #85. #219/#188 have closed #135 and inherited
#15. Closed issues alone do not prove code is present on another integration
branch: #188's latest assessment (#396) still records the #16 prerequisite gap.
#80/#81/#82 remain human Provider migration gates; their dependent Provider,
confinement and secret work is not ready. #197 waits for #124; #125 waits for
#197. #16/#17/#18/#19 and exit #20 remain open.

Selected #124 because capture replay integrity protects existing Evidence and
its downstream Approval/retention chain. Both Artifact indexes returned the
previous version as success even when a repeated capture changed hash, size or
media type. An ingester retry after output bytes changed silently reported the
old hash. This bounded defect can be fixed under accepted ADR-0017/0069 without
inventing remaining recorder shapes, public API semantics or infrastructure.

## Change and architecture review

The existing capture key remains Artifact/Execution/Step/attempt/path. Under
the existing fake mutex or PostgreSQL per-Artifact transaction lock, a replay
now compares hash, size and media type; a mismatch returns `domain.ErrExists`
and an empty version. Exact replay returns the original version and timestamp;
caller Version is ignored, and a fresh attempt can create the next version.
The ingester propagates rejection and returns no successful result. No stored
version is overwritten or appended for the rejected capture.

Scope: both index adapters, port documentation, shared conformance suite,
ingester integration test and documentation. Caller inspection includes the
Evidence conformance/walkthrough and PostgreSQL concurrency/read-scope suites.
No signature, public schema, migration, dependency or configuration changes.
Architecture verdict: sound within accepted boundaries. Platform owns versions
and provenance; domain purity and Worker ownership are unchanged. The simpler
existing replay shortcut loses conflict information, so it cannot protect this
invariant. Transaction rollback and existing locks preserve immutable history.

Per [ADR-0017](../../architecture/adr/0017-content-addressed-artifact-store-filesystem-or-s3.md),
Git owns Project/source history. Amaru ArtifactStore owns immutable
content-addressed output blobs, with version/history/provenance metadata in
PostgreSQL. Filesystem is the compact adapter; RustFS is the reference
S3-compatible backend for self-hosted/Cloud. This slice adds no S3 service or
adapter and makes no RustFS runtime validation claim.

Blob ingest still precedes metadata recording. A rejected replay can leave an
unreferenced blob under existing behavior; garbage collection is not implemented.
Multi-output metadata writes remain separate calls, so a later failure can leave
earlier versions recorded; this slice does not claim batch atomicity. Different
paths remain different capture keys; neither new key semantics nor cross-Project
API authorization is introduced.

## Commands and evidence

Versions: root Go launcher 1.26.0, Platform toolchain 1.27.0, Node v25.7.0,
pnpm 11.5.0, PostgreSQL 17.5. Test PostgreSQL used new disposable data at
`/tmp/amaru-124-artifact-replay-pgdata`, loopback port 55493, test-only trust
role `artifact_test`; this is test setup, not product infrastructure/security.

- #398 exact-head root `make check`: PASS exit 0 with execution access,
  348 TypeScript tests, 346 Go/TS fixture outcomes, 48 SDK tests.
  `go test -race -count=1 ./internal/platform ./internal/adapter/operational`
  from its Platform module: PASS exit 0. Initial sandbox attempts failed on
  denied loopback sockets and a generic Node fixture failure; unchanged-code
  reruns passed. Generic Node failure cause remains unproven.
- Before the fix, from this Platform module,
  `go test -count=1 ./internal/ports/fakes ./internal/artifact -run 'TestFakeArtifactIndex|TestChangedOutputReplayIsRefused'`:
  FAIL exit 1 with execution access: three conflict subcases plus fake/filesystem
  ingester subcases incorrectly returned success. Initial sandbox attempt failed
  on read-only Go cache before execution. Expected error assertion was subsequently
  narrowed from stale-edit `ErrConflict` to existing duplicate-identity `ErrExists`.
- After the fix, `go test -race -count=1 ./internal/ports/fakes ./internal/artifact ./internal/evidence ./internal/coordinator ./internal/store/fsblob ./internal/store/postgres`:
  PASS exit 0, all six packages. Untagged PostgreSQL package is unit evidence.
- `AMARU_TEST_PG=postgres://artifact_test@127.0.0.1:55493/postgres go test -race -count=1 -v -tags pg ./internal/store/postgres -run 'TestArtifactIndexConformance|TestArtifactVersionsAreImmutableInTheDatabase|TestArtifactVersionNumbersHaveNoGapsUnderConcurrentWriters|TestEvidenceReadScope'`:
  PASS exit 0, eight top-level tests, no skips. Includes real PostgreSQL shared
  conflict/replay cases and existing concurrency/snapshot/immutability controls.

- Root `make check`: PASS exit 0 with execution access; 338 TypeScript contract
  tests, 336 identical Go/TS fixture outcomes, 17 SDK tests, Platform tests,
  Protobuf checks, documentation and host/cross-OS/pg-tag vet. Different test
  counts from #398 reflect the separate designated integration bases.
- Final `make docs-check` and `git diff --check`: PASS exit 0.
- `trail check --base 1e5c9e6d93fa1e0acdc1c0b0c99d84e3d5f8320e --json`:
  PASS exit 0, no conflicts or overlaps. Linux-first matches agree with the
  accepted OS scope; the Grant replay lexical match is unrelated to Artifact
  capture and adds no Grant expiry claim. Optional deep analysis was not run.
- `/tmp/amaru-335-postgres/bin/pg_ctl -D /tmp/amaru-124-artifact-replay-pgdata -m fast -w stop`:
  PASS exit 0; only this session's disposable server was stopped. Files preserved.
Logs: `/tmp/m3-398-review-{check,race}-access.log`,
`/tmp/m3-artifact-replay-before-access.log`, `/tmp/m3-artifact-replay-after.log`,
`/tmp/m3-artifact-replay-pg.log`.

## Remaining M3 gates

This is unit/integration evidence for a bounded capture invariant, not full
ART-002 or #19 acceptance. Remaining checks/diff/log/workspace producers,
authorized versioned public Evidence read, blob availability, full audit query
and Approval binding remain open. #154 still needs production launch/root
reporting; #92 startup/CLI, #170 production diagnostic consumption, #191 scope
integration and #219 typed/file change Events remain incomplete. ADR-0070 is
proposed, not accepted. Provider migration gates remain human decisions.

#20 still needs one integrated Linux candidate with behavior/contracts, crash
injection, ungranted Capability refusal, restart-safe Approvals, Codex/OpenCode
confinement refusal, core identifier audit and a recorded real Linux headless
Provider execution. Native macOS/Windows are not validated, best effort for M3.
No issue closure or acceptance-checkbox change is made by this partial slice.
