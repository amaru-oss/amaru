# Project query identity binding — Linux

Date: 2026-11-14. Runner: Codex for Ricardo Lüders. Linux x86_64,
kernel `7.2.11-100.fc43.x86_64`; physical/VM qualification not verified.
Go launcher 1.26.0, Platform toolchain 1.27.0, Node v25.7.0, pnpm 11.5.0.
Local results, not hosted CI.

## Selection and refreshed queue

Refreshed origin and all 25 open M3 issues, including each native blocked-by
relationship and parent. Refreshed open PR queue: empty. PR #411 merged into
#18 at `9dcc4cab57be78a534183440b8c78c22b997cccc`. Selected #191 on designated
base `feat/18-wp-m3-06-public-api-events`; direct #120 and inherited #15 are
closed, and their implementation is on this base. Accepted ADR-0062 is present.
Branch `fix/191-query-session-identity` uses only existing `/tmp/amaru-124-api`.
No worktrees created/removed; existing untracked `.trail/.gitignore` preserved.
Contributor: @rluders through Codex. No issue closure or Project-board mutation.

Snapshot of all open M3 relationships (closed blockers remain relevant for base
availability; native relationships are authoritative):

| Open issues | Native parent | Native blockers (state) |
| --- | --- | --- |
| #219 | #18 | #135 closed |
| #197 | #16 | #124 open, #195 closed |
| #191 | #18 | #120 closed |
| #190 | #17 | #127 open, #113 open |
| #189 | #17 | #113 open |
| #188 | #16 | #135 closed |
| #170, #92 | #18 | #84 closed, #83 closed |
| #163, #131, #128, #127, #113, #112 | #17 | #80, #81, #82 open |
| #154 | #19 | #85 closed, #120 closed |
| #125 | #19 | #197 open |
| #124 | #19 | #119 closed, #123 closed |
| #80, #81, #82 | #17 | #8 closed |
| #20 | none | #16, #17, #18, #19 open |
| #16, #17, #18, #19 | none | #15 closed |

#18's ready prerequisite status does not complete #92 startup/`amaru open`,
#170 product-channel logging, #219 typed changes/file detection/conflicts, or
#191 Work/definition/Provider/secret/product acceptance. This reproduced read
boundary defect is a bounded part of #191's isolation acceptance, with no new
decision required. #124 remains available for bounded work on its own #19 base;
#154 still owes actual Worker-entry confinement evidence. #188's #135 code is
absent from its #16 base (assessment retained at `cd5c17d`); it needs integration
prerequisites, not speculative replacement code. #197 then #125 remain blocked.
Provider implementation/secret work retains #80–82 owner migration gates.
#20 requires all four parents and full Linux headless/real-Provider acceptance.
Linux is required; native macOS/Windows remain best effort for M3.

ADR gates refreshed against integration decision indexes: accepted ADR-0062 and
0068 on #18, accepted ADR-0069 on #19. ADR-0070 Work commands on #18 and
ADR-0071 public Evidence reads on #19 remain proposed; neither is implemented
here. #19's authored-assets Q70 remains an unresolved design gate. #18 remains
at its designated head; origin/main `d01934e` has two M4-marker documentation
commits not yet in #18. No shared integration branch is changed by this slice.

## Change and boundary review

Existing queries rechecked valid authority after storage IO but discarded the
principal returned by both checks. A different valid owner could receive results
read under the original request identity. Execution (and its delegated pinned
Definition), Approval, Events, Evidence, Settings, ArtifactVersions and
ArtifactContent now compare original and final human IDs. Evidence and content
capture the administrator before their existing nested authorized ownership reads;
those checks remain in place. Rejected content closes the already opened reader.
Errors still return zero results, and existing storage/missing/foreign-resource
error precedence is retained. No mutation transaction semantics are changed.

Scope: three coordinator source files, the existing shared read-authority test,
Project access/validation guides and this record. No callers, public signatures,
fixtures, config, exports, routes, dependencies, infrastructure or licenses change.
Trail call tracing covers delegated Definition/Evidence/Artifact access and the
private ownership helper's mutation callers; mutation behavior is unchanged.
Boundary review: **sound** under accepted ADR-0062. Platform retains authority;
Worker/Client/domain boundaries remain unchanged. Accepting any valid replacement
owner would not preserve request identity. No new ADR or owner decision is needed.

The real compact Auth has a stable local-administrator ID. Replacement tests
exercise the trusted authentication callback; they do not establish an exploit
through a shipped public route. This slice does not cover Project inspection,
list/attach or all mutation paths, nor continuous session validity. Final checks
are not atomic with response delivery, database commit or continued blob reads.

## Reproduction and validation

The shared `TestProjectReadAuthorityDuringIO` matrix now has 64 cases: successful
reads, revocation, cancellation, replacement identity, storage failure and failure
with replacement across Project lookup, Execution, Approval, Event/Evidence/settings
IO, Artifact index/content and nested ownership reads. Two Project-lookup error
cases are excluded; lookup error precedence has its own regression.
Opened content is closed after denial; positive content is caller-closed. Tests
reuse existing fakes/probes and need no real agent or paid API.

Original implementation plus regression, from `services/platform`:
`TMPDIR=/home/rluders/.cache/amaru-query-validation/tmp GOFLAGS=-p=1 go test -count=1 -run '^TestProjectReadAuthorityDuringIO$' ./internal/coordinator`
FAIL, exit 1: exactly ten replacement-identity cases, including an unclosed
returned reader. Log `/tmp/191-query-before.log`. The initial `/tmp` build failed
with disk quota exceeded before tests; a separate build-temp directory resolved
it without removing any existing work or caches. An intermediate candidate build
caught a misplaced content guard; corrected before successful validation.

Candidate checks:

- From `services/platform`,
  `TMPDIR=/home/rluders/.cache/amaru-query-validation/tmp GOFLAGS=-p=1 go test -race -count=100 -run '^TestProjectReadAuthorityDuringIO$' ./internal/coordinator`:
  PASS, exit 0, 64 cases ×100; `/tmp/191-query-stress.log`.
- From `services/platform`,
  `TMPDIR=/home/rluders/.cache/amaru-query-validation/tmp GOFLAGS=-p=1 go test -race -count=1 ./internal/coordinator ./internal/adapter/httpapi ./internal/adapter/localauth ./internal/arch`:
  PASS, exit 0, all four packages; `/tmp/191-query-race.log`.
- Root `TMPDIR=/home/rluders/.cache/amaru-query-validation/tmp GOFLAGS=-p=1 make check`:
  PASS, exit 0; 348 TypeScript contract tests, 346 identical Go/TypeScript fixture
  outcomes, 48 SDK tests, Platform/domain/architecture tests, Protobuf checks,
  host/cross-OS vet, PostgreSQL-tag vet and docs; `/tmp/191-query-check.log`.
  Repeated after refining error injection: PASS exit 0,
  `/tmp/191-query-final-check.log`.
- Root `TMPDIR=/home/rluders/.cache/amaru-query-validation/tmp GOFLAGS=-p=1 make docs-check`:
  PASS exit 0 (`doccheck: ok`), `/tmp/191-query-docs.log`.
  `git diff --check`: PASS, exit 0.
- `trail build`: PASS, exit 0. `trail check --base origin/feat/18-wp-m3-06-public-api-events`:
  exit 1, nine lexical conflicts in two documentation files, with the untracked
  evidence record outside this check. Reviewed individually: four Project-guide
  matches concern captured clone binding, direct Workflow post-source auth,
  replacement-actor refusal and auth-before-context-error ordering; all remain
  intact. Five validation-guide matches concern mutation actor preservation,
  hijacked connection shutdown, startup cancellation ordering, SQL panic cleanup
  and receive-pump joining; this slice changes none of that lifecycle/storage
  behavior. Advisory matches likewise change no clone path, pre-v1 migration,
  scheduling or PostgreSQL reliability claims. This is an explained nonzero
  semantic check, not a passing Trail check.

Evidence level: **unit-tested** for identity switching, using fake stores/readers
and a trusted authentication callback. Existing HTTP/auth integration suites also
passed, without establishing this scenario through real session/browser traffic.
No new PostgreSQL runtime, native non-Linux, real Provider, product startup/process
restart or new public Workflow/Evidence/Artifact route validation. Cross-OS vet
is not native evidence. #191/#18/#20 and M3 remain open; no full acceptance,
merge or proposed-decision implementation is claimed.

## Same-Project record identity follow-up (2026-11-20)

On the #18 base at `2e7d76f`, the Execution and Approval ownership
lookups accepted a same-Project record carrying a different ID.
They now require both the requested record ID and the Project to match
before returning data or using the result as an ownership check.

- From `services/platform`, `go test -count=1 ./internal/coordinator -run 'TestProjectReadAuthorityDuringIO/(execution|approval)/different_record'`
  failed before the fix (exit 1). Both cases returned data and a nil error.
- From `services/platform`, `go test -race -count=1 ./internal/coordinator`
  passed after the fix (exit 0), including the two new cases.
- From the repository root, `make check` passed (exit 0).

The mismatch cases are unit-tested with a fake store. They add no
PostgreSQL runtime, real Provider or native non-Linux evidence.

## Evidence bundle identity follow-up (2026-11-20)

The Project-scoped Evidence read now verifies the returned bundle's
Execution ID and Project before releasing the bundle. The existing
authorization check after the read still runs before the response.

- From `services/platform`, `go test -count=1 ./internal/coordinator -run 'TestProjectReadAuthorityDuringIO/^evidence$/(different_record|foreign_bundle)'`
  failed before the fix (exit 1). A different Execution in the same
  Project and the requested ID in a different Project both returned
  Evidence with a nil error.
- From `services/platform`, `go test -race -count=1 ./internal/coordinator`
  passed with the fix (exit 0).
- From the repository root, `make check` passed (exit 0).

The mismatch cases are unit-tested with a fake Evidence reader.

## Event page scope

The Project access boundary returns an empty page and ErrNotFound if the Event
journal returns a record from a different Project or outside the requested
Execution filter. The fake-backed test reproduces both mismatches and verifies
that the response withholds the entire page, including its cursor.

## Follow-up

The Project Execution read now rejects a store response containing a Step from a different Execution, even if the parent Execution matches. A fake store test verifies that the read returns no Execution or Steps.
