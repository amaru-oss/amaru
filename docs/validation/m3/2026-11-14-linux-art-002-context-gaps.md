# ART-002 missing context reporting — Linux

Date: 2026-11-14. Runner: Codex for Ricardo Lüders. Local Linux x86_64;
machine kind not independently verified. Local results are not hosted CI.
Refs #124, parent #19 and M3 exit #20; all remain open.

## Exact-head review and merge of #403

PR [#403](https://github.com/rluders/amaru/pull/403) and its isolated worktree
`/tmp/amaru-191-settings-session-io` both matched
`24bed97d13da505482ddf9c6ca59ed23d37583d8`. Tracked work was clean;
untracked `.trail/cache/` was preserved. The regression and connected Settings,
ProjectAccess and Auth paths were inspected with no actionable finding.
GitHub reported MERGEABLE/CLEAN with zero check runs and zero legacy statuses.

Fresh exact-head `GOFLAGS=-p=1 make check` passed, exit 0, including 348
TypeScript contract tests, 346 identical Go/TypeScript fixture outcomes and
48 SDK tests. Log: `/tmp/pr403-make-check-unsandboxed.log`.
From `services/platform`,
`go test -race -count=1 ./internal/adapter/httpapi ./internal/adapter/localauth`
passed both packages, exit 0; log `/tmp/pr403-race.log`.
`git diff --check HEAD^ HEAD` passed. The initial sandbox full check stopped
at the Node fixture subprocess; the unchanged check passed outside the sandbox.

The [COMMENT review](https://github.com/rluders/amaru/pull/403#pullrequestreview-5905983431)
is pinned to the exact requested head; author `rluders` cannot self-approve.
`gh pr merge 403 --repo rluders/amaru --merge --match-head-commit 24bed97d13da505482ddf9c6ca59ed23d37583d8`
succeeded. GitHub readback confirmed MERGED at
`f732a09c7756f042b6843e570ab5c36d0ada1fa9`. Fetch and local inspection verified
the reviewed head is its second parent and the trees are identical. Remote #18
also resolved to that merge. No issue was closed.

## Refreshed M3 queue and selection

Read all 25 open issues in `M3 Platform core`, their native blocked-by
relationships, candidate acceptance, parent blockers, remote refs and open PRs.
No competing PR remained after #403 merged.

- #124's native #119/#123 and inherited #19 blocker #15 are closed; its
  existing context, Artifact and Reader implementation is on designated #19
  base `1de18c0265608e0d77738edbf2f9e5588f20b523`. #124 blocks #197,
  which blocks #125, so bounded accepted Evidence work has downstream impact.
- #191/#92/#170 and #219 have closed native and inherited prerequisites
  on #18. Full product acceptance remains open. Work ADR-0070 and public
  Evidence ADR-0071 remain proposed; neither is implemented here.
- #188's native #135 is closed, but its designated #16 base still lacks
  the ordered Event port. Closed issue state alone does not supply that code.
- Provider migration decisions #80/#81/#82 remain open owner gates.
  #112/#113/#127/#128/#131/#163 and downstream #189/#190 remain blocked.
  #154's closed prerequisites do not establish production Worker composition.
- #16–#19 and exit #20 remain open. No acceptance checkbox, issue closure
  or Project status change is inferred from a bounded child merge.

Created branch `feat/124-evidence-context-gaps` in new isolated worktree
`/tmp/amaru-124-context-gaps` from the designated #19 base. Existing worktrees
and uncommitted files were preserved. The explicit missing-context limitation
in the earlier multi-Step walkthrough supplies the concrete next slice.

## Change and boundary review

The pure internal assembler sets `contextGap` on each Step without a recorded
context and emits one `context` bundle summary if any are missing. Recorded
contexts have no gap; an Execution without Steps has no context gap. Reporting
does not depend on running versus terminal state. Existing representation
hashing includes these fields; stored Approval Evidence is never changed.

This follows accepted ADR-0069's requirement to expose missing Evidence. It
does not infer pending, unavailable, failed or not-applicable causes from an
absent record. The simpler nullable field alone failed to explicitly report
absence in the existing gap inventory. The existing Usage-gap pattern avoids
a new recorder, service, abstraction or completeness state machine.

Boundary review: domain remains pure; Reader/store composition is reused;
there are no new dependencies, infrastructure, public contracts, capture
protocols, persistence writes or Policy/Approval lifecycle semantics. No new
significant architecture decision is taken. Per-Step domain tests, the shared
fake/PostgreSQL Reader suite and the multi-Step coordinator walkthrough cover
the affected consumers.

## Validation

The initial focused test had an incorrect Go state type name and failed to
compile; after correction, the unchanged assembler failed the actual regression:
`go test -count=1 -run '^TestExecutionEvidenceReportsMissingContext$' ./internal/domain`
returned exit 1, reporting zero context gaps for a running Execution with a
missing context. Module-scoped checks below validate the corrected implementation.

- From `services/platform`,
  `go test -race -count=1 ./internal/domain ./internal/evidence ./internal/coordinator ./internal/ports/fakes`:
  PASS, exit 0, all four packages; `/tmp/124-context-gaps-race-final.log`.
- Root `GOFLAGS=-p=1 make check`: PASS, exit 0, including 338 TypeScript
  contract tests, 336 matching Go/TypeScript fixture outcomes, 17 SDK tests,
  all Go suites, cross-OS/pg-tagged vet, Protobuf and documentation checks.
  Log: `/tmp/124-context-gaps-check-final.log`. An earlier run stopped on the
  new documentation link before its target record was written; adding this
  record resolved it. Initial module-scoped commands also used incorrect
  formatting paths and encountered the sandbox's read-only Go cache; paths
  were corrected and the reported checks ran with cache access.
- Final `make docs-check` and `git diff --check` results are in the delivery PR.

Evidence limits: unit tests and fake-backed integration only. PostgreSQL runtime,
real Provider, public API, product server and native macOS/Windows are not
validated by this slice. The shared PostgreSQL suite is compiled by `make check`;
its assertions are not runtime evidence without a database run. Missing
definitions, remaining recorders, complete completeness classification,
authorized public reads and full #124/#19/M3 acceptance remain open.
