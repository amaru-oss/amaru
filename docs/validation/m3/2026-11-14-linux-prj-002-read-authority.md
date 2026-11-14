# Project query authority during IO — Linux

Date: 2026-11-14. Runner: Codex for Ricardo Lüders. Local Linux x86_64,
kernel `7.2.11-100.fc43.x86_64`; physical/VM qualification not verified.
Tools: Go launcher 1.26.0, Platform toolchain 1.27.0, Node v25.7.0, pnpm 11.5.0.
These are local checks, not CI or real Provider validation.

## Exact-head proposal review and merge

PR [#401](https://github.com/rluders/amaru/pull/401) was reviewed at
`68ebd8cb4735255b08e1e0e27bd0cc515b75c81b` against
`5cf51aa0eec42bbbb3b4e59ec38d4fcacf3ff453`. The owner reported boundary review
passed and instructed merge as proposed. No actionable finding. The
[COMMENT review](https://github.com/rluders/amaru/pull/401#pullrequestreview-5905912957)
is pinned to that head; authenticated author `rluders` cannot self-approve.
`gh pr merge 401 --merge --match-head-commit 68ebd8cb4735255b08e1e0e27bd0cc515b75c81b`
succeeded; GitHub readback verified MERGED at
`1de18c0265608e0d77738edbf2f9e5588f20b523` into #19's integration branch.

`make docs-check`, `git diff --check` and final
`GOFLAGS=-p=1 make check` passed on the exact head (exit 0). The first full
attempt failed in the sandbox's Node fixture runner; the unchanged 338-test
suite passed outside it. The next full attempt exhausted the user's `/tmp`
quota in real Git tests. Only disposable Node 25 compile-cache data was removed;
the full retry passed. Log: `/tmp/pr401-review-check-quota-retry.log`.
ADR-0071 remains proposed, Q69 open; no route was implemented and no issue closed.

## Live M3 selection

Refreshed the live `M3 Platform core` milestone (25 open issues), open PRs,
native dependencies and designated remote bases after #401. No competing open
PR remained. Native #191 blocker #120 and inherited #18 blocker #15 are closed;
accepted #83/#84 prerequisites are also closed and on the #18 base. #219's #135
is closed and present there. #92/#170 retain product composition gaps; #219
needs actual authoritative definition mutations and reconciliation, not another
notification-only layer. #191 Work contracts still require the separate proposed
ADR-0070 decision. Existing Project reads supplied the concrete executable
security gap selected here under accepted ADR-0062, independently of ADR-0071.

#197 remains blocked by open #124 (#195 closed); #125 by open #197. #188's
native #135 is closed, but its implementation remains absent from the designated
#16 base and main; it cannot start on prerequisite closure alone. Provider
migration #80/#81/#82 remain human gates, and #154 lacks production Worker-entry
composition. All blocked items stay open. No new decision, infrastructure,
dependency, prototype import, acceptance checkbox or Project status is introduced.

Created isolated worktree `/tmp/amaru-191-read-authority`, branch
`fix/191-project-read-authority`, from #18 integration head
`39386f7e9cd2782075d1cf865955a39ccef71a26`. The first checkout failed due to the
same `/tmp` quota; retry after clearing disposable compile cache succeeded.
Existing worktrees, tracked/untracked source work and the original checkout
were preserved.

## Reproduction and change

Before editing runtime code, `go test -count=1 -run TestProjectReadAuthorityDuringIO ./internal/coordinator`
failed with 16 deterministic cases: revoked/canceled reads returned data, and
some storage-error paths returned partial results. Log:
`/tmp/191-read-authority-before.log`. The matrix also retains positive authorized
reads. Hooks revoke authority or cancel the context during the actual fake
storage call, without timing sleeps.

`ProjectAccess.authorize` now rechecks its live administrator after Project IO.
Execution, Approval, existing internal Evidence, Event replay and settings query
wrappers recheck Project authorization before returning a successful value and
zero results on storage errors. Definition inherits the Execution guard. Existing
Workflow/Artifact post-IO checks remain. Internal scheduling and authoritative
storage APIs, mutation commits, contracts and SDK need no changes. Read callers
and neighboring tests were inspected before the edit; no signature changed.

Boundary review: follows accepted ADR-0062's common application boundary and
existing Workflow/Artifact pattern. No new authority or architecture decision.
This does not make the final check atomic with response delivery, implement
ADR-0071, or authorize a public Evidence snapshot. Mutation authority during
storage transactions is outside this query fix.

## Validation

- Focused `go test -race -count=1 -run 'TestProject' ./internal/coordinator`:
  PASS, exit 0. New matrix: 23 cases covering authorized reads, revocation,
  cancellation and storage failures across six read stages.
- Root `GOFLAGS=-p=1 make check`: PASS, exit 0. All required checks ran:
  348 TypeScript contract tests, 346 matching Go/TypeScript fixture outcomes,
  48 SDK tests, Platform/domain/architecture tests, Protobuf, host and cross-OS
  vet, PostgreSQL-tag vet and docs. Log: `/tmp/191-read-authority-check.log`.
- From `services/platform`, `go test -race -count=1 ./internal/coordinator ./internal/adapter/httpapi ./internal/adapter/localauth`:
  PASS, exit 0, all three packages. Log: `/tmp/191-read-authority-race.log`.
- Final documentation and whitespace checks are recorded in the PR.

Evidence level: **unit-tested**, using deterministic storage/authentication fakes.
No PostgreSQL runtime, native macOS/Windows, real Provider, public Evidence API,
full #191/#18 or M3 acceptance is claimed. Those delivery gates remain open.
