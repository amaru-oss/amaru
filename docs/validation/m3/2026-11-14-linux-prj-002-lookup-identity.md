# Project lookup preserves request identity — Linux

Date: 2026-11-14. Runner: Codex for Ricardo Lüders. Local Linux x86_64,
kernel `7.2.11-100.fc43.x86_64`; physical/VM qualification not verified.
Go launcher 1.26.0, Platform toolchain 1.27.0, Node v25.7.0, pnpm 11.5.0.
Local evidence only, not CI.

## Refreshed selection and integration queue

Refreshed origin, M3 roadmap, all 25 open M3 issues, their native blocked-by and
parent relationships, #191/#18 discussion and the open PR queue before selecting.
No PR was open. PR #409 is merged into #18 at
`3ae9047bd2c54aa8a634ced2326e9142c341dc90`. Remote heads observed:

| Designated base | Head | Remaining queue |
| --- | --- | --- |
| `main` | `d01934ea51a2839d3f957d23c9814a598a056cba` | Parent integration and eventual exit review |
| #16 `feat/16-wp-m3-04-policy-grants-and` | `cd5c17d740315bb471f735b60257df2cab4bddbe` | #188 audit acceptance; #197 blocked by open #124 |
| #17 `feat/17-wp-m3-05-providers-and-context` | No remote branch observed | #80–82 owner gates; dependent #112/#113/#127/#128/#131/#163; #189/#190 depend on those |
| #18 `feat/18-wp-m3-06-public-api-events` | `3ae9047bd2c54aa8a634ced2326e9142c341dc90` | #92/#170/#191/#219; selected bounded #191 guard |
| #19 `feat/19-wp-m3-07-worktrees-configuration-scopes` | `025a84defbb2c40fedae11ecbf70a5d2f9fd83a6` | #124 Evidence, #154 production-entry acceptance; #125 blocked by #197 |

#16–19 each have closed native blocker #15. #191 has closed blocker #120;
#170/#92 have closed #83/#84 and #219 has closed #135. These closures do not
complete parent acceptance. #20 remains blocked by all four open parents.
#188's closed native #135 prerequisite alone does not establish code ancestry on
#16; that separate queue was not selected. The #15 integration completion
`4213a55` and #120 merge `755ffd7` are ancestors of the selected #18 base.

Fresh isolated worktree: `/tmp/amaru-191-definition-session`, initially created as
`fix/191-definition-session-identity` while inspecting the Workflow path. The
smaller shared Project lookup defect was selected instead. Existing worktrees,
Q70 and unrelated untracked files were preserved. No issue or Project-board
state was changed. ADR-0070 Work commands remain proposed and blocked; #80–82
owner migration gates and infrastructure without accepted ADRs are excluded.
The roadmap's real-agent/headless/crash/refusal and full API exit requirements
remain open. Linux is required; native macOS/Windows remain best effort under
the existing owner decision recorded in the roadmap and Trail learning.

## Change and review

Under accepted ADR-0062 and the owner requirement recorded in #409's evidence,
`ProjectAccess.authorize` now retains the initial human identity across the
Project-store lookup. A successful lookup followed by a different authenticated
human ID returns empty authority and `ErrNotPermitted`. Previously the helper
returned the replacement principal; reads could proceed and Submit could create
an Execution attributed to that replacement. Existing role/sponsor validation,
revocation/cancellation and storage/missing/foreign error precedence remain.

Scope: the shared helper, regression coverage and affected Project access and
validation docs. Its callers include Execution/Approval, Events, Evidence,
settings, Artifacts, Workflow loading, Submit and existing human mutations.
No exports, fixtures, configuration, contracts, routes or dependencies change.
Architecture review: **sound** within accepted ADR-0062. Platform retains
state and authority; trusted scheduler APIs and domain purity are unchanged.
Checking individual mutations alone misses initial actor replacement during the
common lookup. The helper check is smaller and covers all existing callers.

This compares identity only across each Project lookup. It does not promise
identity binding across all separate resource reads/operations, continuous
stream revocation, or atomic authorization with database commit/response delivery.
The production compact Auth uses a stable local-administrator ID; the regression
uses its supported trusted authentication callback, not a claimed exploit through
a shipped product route. No storage semantics or persistence changes.

## Reproduction and validation

`TestProjectLookupPreservesIdentity` runs read and Submit with seven lookup
outcomes: authorized, changed identity, revoked, cancelled, storage failure,
missing and foreign. The fake changes identity inside `ProjectStore.Get` before
it returns, without sleeps or scheduling assumptions. Denied cases require zero
returned data, false replay and no downstream resource read, coordinator command
lookup or commit. Error cases also revoke identity to verify precedence. The
successful Submit checks original-actor attribution; normal Policy remains active.

Before runtime edits, from `services/platform`,
`go test -count=1 -run '^TestProjectLookupPreservesIdentity$' ./internal/coordinator`
failed exactly the two replacement-identity cases (exit 1), log
`/tmp/191-lookup-before.log`.

Final checks:

- `go test -race -count=100 -run '^TestProjectLookupPreservesIdentity$' ./internal/coordinator`
  from `services/platform`: PASS, exit 0, 14 cases ×100; `/tmp/191-lookup-stress.log`.
- Root `GOFLAGS=-p=1 make check`: PASS, exit 0; 348 TypeScript contract tests,
  346 identical Go/TypeScript fixture outcomes, 48 SDK tests, Platform/domain/
  architecture tests, Protobuf checks, host/cross-OS vet, PostgreSQL-tag vet and
  docs. `/tmp/191-lookup-check.log`.
- From `services/platform`, `GOFLAGS=-p=1 go test -race -count=1 ./internal/coordinator ./internal/adapter/httpapi ./internal/adapter/localauth`:
  PASS, exit 0, all three packages; `/tmp/191-lookup-race-final.log`.
  The initial nonserial race run passed coordinator/auth but failed linking HTTP
  with `disk quota exceeded`; a task-owned `/tmp` cache attempt also exhausted
  the quota during compilation. The serial rerun with the original cache after
  the full check completed passed. No existing caches or worktrees were removed.
- Final root `make docs-check` and `git diff --check`: PASS, exit 0.
- `trail build`: PASS. `trail check --base origin/feat/18-wp-m3-06-public-api-events`:
  exit 1, five lexical conflicts in the tracked documentation diff, reviewed
  individually. Two match the changed-human-identity learning: this patch
  enforces it rather than allowing replacement. One matches native clone path
  checks: no path validation or atomic registration changes. One matches
  transport-auth error precedence: `administrator` still authenticates before
  checking cancellation. One matches Workflow post-source authorization: that
  check is unchanged. These are explained conflicts, not a passing Trail check.

Evidence level: **unit-tested** using fake storage/authentication and actual
coordinator Submit/Policy logic. Broader HTTP/auth race suites are regression
checks, not new real-session identity-switch evidence. No PostgreSQL runtime,
real Provider, product process/bootstrap, native macOS/Windows or full
#191/#18/M3 acceptance is claimed. Cross-OS/tagged vet is compilation evidence.

The published branch is `fix/191-project-lookup-identity`; the worktree path
retains its initial name. No merge or issue closure is authorized by this slice.
