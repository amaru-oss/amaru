# ART-002 recorded context integrity — Linux

Date: 2026-11-14. Runner: Codex for Ricardo Lüders. Local Linux x86_64;
local results are not hosted CI. Refs #124, parent #19 and exit #20; all remain open.

## Exact-head review and merge of #405

Reviewed PR [#405](https://github.com/rluders/amaru/pull/405) in its separate
worktree `/tmp/amaru-124-definition-integrity` at exact head
`f7243614e2be21fee398f3fe9d2bf0e830e43976`, base #19
`649f265267160a022a804dd3e710c3308ce0697d`. No actionable issue was found in
the limited validation change, validator, Reader, callers, tests or documentation.
Tracked files were clean; generated untracked `.trail/cache/` was preserved.

Fresh local `GOFLAGS=-p=1 make check` passed, exit 0: 338 TypeScript contract
tests, 336 identical Go/TypeScript fixture outcomes, 17 SDK tests and required
Go/vet/Protobuf/docs checks. From `services/platform`,
`go test -race -count=1 ./internal/domain ./internal/evidence ./internal/ports/fakes ./internal/coordinator`
passed all four packages, exit 0. `git diff --check` over the exact base/head
passed. Logs: `/tmp/amaru-405-check.log` and `/tmp/amaru-405-race.log`.

GitHub reported OPEN, non-draft, MERGEABLE/CLEAN, zero check runs and zero
legacy statuses. No hosted CI success is inferred. The authenticated author
`rluders` cannot self-approve; the
[COMMENT review](https://github.com/rluders/amaru/pull/405#pullrequestreview-5906211928)
is pinned to the exact head. The guarded command
`gh pr merge 405 --repo rluders/amaru --merge --match-head-commit f7243614e2be21fee398f3fe9d2bf0e830e43976`
succeeded. Readback confirmed MERGED at
`b8351f796658072e52789f70f1ec50b3ee87d202`; its parents are the specified base
and head, its tree equals the reviewed head, and remote #19 matches the merge.

## Queue refresh and next bounded task

Refreshed all 25 open M3 issues, native blocked-by relationships, parents,
candidate acceptance, open PRs and remote integration refs. No competing open
PR remained after #405. Native and inherited prerequisites for #124 are closed,
and the Reader/context/artifact code is present on #19. Selected this context
integrity slice because it reuses accepted validation inside the existing
boundary and #124 blocks #197, then #125. Created isolated worktree
`/tmp/amaru-124-context-integrity` from merge `b8351f7`; preserved other worktrees
and dirty files. No issue closure or Project-board mutation is claimed.

- #191/#92/#170/#219 have closed prerequisites and implementation on #18
  (`f732a09c7756f042b6843e570ab5c36d0ada1fa9`); complete product composition,
  Work/change propagation and acceptance remain open. ADR-0070 remains proposed.
- #188 has closed #135, but its ordered Event implementation `fa8bb47` is still
  absent from #16 (`cd5c17d740315bb471f735b60257df2cab4bddbe`). Bring reviewed
  prerequisite code onto that base before claiming readiness. Owner: Ricardo Lüders.
- Provider migrations await owner gates #80/#81/#82. Their dependent Provider
  work and #189/#190 remain blocked. #154 still lacks production Worker launch
  composition and enforced-root reporting; closed dependencies do not prove it.
- ADR-0071 public Evidence contract awaits owner approval; no new public route,
  schema or authorization promise is introduced here. Recorders, completeness
  classification and full #124/#19/M3 acceptance remain open.

Git remains source management. RustFS is a candidate for Artifact blob storage
(assets and deliverables), not a source-management replacement. Any infrastructure
choice needs an approved ADR; this change introduces no storage implementation.

## Regression and implementation

The assembler accepted a recorded context without calling `StepContext.Validate`.
The new domain regression failed before the fix for wrong hash, empty document
with matching hash, unknown format and missing timestamp; the Reader regression
also failed because it returned a successful bundle. Command from `services/platform`:
`go test -count=1 -run '^(TestExecutionEvidenceValidatesContext|TestBundleRejectsDamagedContext)$' ./internal/domain ./internal/evidence`.
Exit 1, log `/tmp/amaru-124-context-integrity-regression.log`. The initial sandbox
attempt could not write the Go build cache; reproduction used normal cache access.

Assembly now calls the existing validator after checking context identity and
uniqueness. Invalid data returns `ErrInvalid` and no bundle; valid context is
preserved, and absent context remains a gap. The shared domain fixture uses a
matching hash, and the digest test updates the document hash with its edit.
Existing shared fake/PostgreSQL tests already store valid assembled contexts.

Boundary assessment: sound as a small in-component validation fix. Domain remains
pure; no dependency, contract, persistence write, infrastructure or Approval change.
Copying damaged records would retain misleading Evidence. This check reuses the
existing policy rather than inventing a new context parser or item verification.

## Validation and limits

- From `services/platform`,
  `go test -race -count=1 ./internal/domain ./internal/evidence ./internal/ports/fakes ./internal/coordinator`:
  PASS, exit 0, all four packages. Log: `/tmp/amaru-124-context-integrity-race.log`.
- Root `GOFLAGS=-p=1 make check`: PASS, exit 0: 338 TypeScript contract tests,
  336 identical Go/TypeScript fixture outcomes, 17 SDK tests and required
  Go/vet/Protobuf/docs checks. Log: `/tmp/amaru-124-context-integrity-check.log`.
- Final `make docs-check` and `git diff --check` results are recorded in the PR.

Unit and fake-backed integration evidence does not establish PostgreSQL runtime,
real Provider, product API, native macOS/Windows or complete ART-002 acceptance.
Context authenticity and consistency of individual Items with Document remain
outside the existing validator and are not claimed.
