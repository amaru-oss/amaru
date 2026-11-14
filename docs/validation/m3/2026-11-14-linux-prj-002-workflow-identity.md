# Committed Workflow identity binding — Linux

Date: 2026-11-14. Runner: Codex for Ricardo Lüders. Local Linux x86_64,
kernel `7.2.11-100.fc43.x86_64`; physical/VM qualification not verified.
Go launcher 1.26.0, Platform toolchain 1.27.0, Node v25.7.0, pnpm 11.5.0.
Local evidence only, not CI.

## Refreshed selection and scope

Refreshed origin, all 25 open M3 issues, native blocked-by and parent
relationships, #191/#18 bodies and discussion, and the open PR queue (empty).
#410 merged at #18 integration head `4bb123c26378ca9c36b0e29198b718b7893990bc`.
The designated base is `feat/18-wp-m3-06-public-api-events`.
#191's native #120 and inherited #15 are closed and their code is present.
#92/#170 have closed #83/#84 and #219 has closed #135; they remain open for
acceptance. #16–19 have closed #15; #20 depends on those four open parents.
#197 waits on #124 and #125 waits on #197. #80–82 remain owner migration gates
for dependent Provider/secret work. Work command ADR-0070 remains proposed.
No fallback slice or new architectural decision was necessary.

Used existing `/tmp/amaru-124-api`, fresh branch
`fix/191-workflow-session-identity` from the refreshed integration head.
Preserved untracked `.trail/.gitignore`. No worktree was created or removed;
the replacement checkout under `/home/rluders/Projects/rluders/amaru-m3-next-slice`
was not changed. Issues stay open; no board mutation or merge is performed.
Linux remains required; native macOS/Windows are best effort under the recorded
M3 owner decision.

## Change and boundary review

`ProjectAccess.workflow` now captures the authorized principal before source IO
and compares its human ID with the post-source principal. It returns that
principal alongside the validated definition/provenance to its two private
callers. `Workflow` discards the principal; `RunWorkflow` rechecks and compares it
before calling coordinator Submit with the original actor and selected Project.
This preserves the final pre-submit authorization without allowing a new
administrator to become the actor between loading and command lookup.

Recorded post-source rules remain: authorize even after source failure, withhold
all returned content/provenance after authorization loss, prioritize authorization
errors, then propagate source errors or validate content. Existing Project lookup
error precedence, manual trigger, Policy, pinning, command keys and atomic
Execution/Step/outcome storage are unchanged. The shared Project lookup fix in
#410 alone did not bind identity across separate lookups/source IO.

Scope: the Workflow helper and both callers, its regression tests, Project access
guide, validation guide and this record. No public signature, route, fixture,
configuration, export, dependency, infrastructure, storage or license change.
Architecture review: **sound** under accepted ADR-0062. Platform retains authority;
domain purity and Worker/Client boundaries remain unchanged. Merely checking that
the replacement principal is an owner is insufficient to preserve request identity.

The real compact Auth uses a stable local-administrator human ID. Replacement
identity tests exercise the supported trusted authentication callback; they are
not evidence of an exploit through a shipped public Workflow route. Checks are
not atomic with commit/response delivery and do not guarantee continuous session
validity. Other query paths and full #191/#18/M3 acceptance are outside scope.

## Reproduction and validation

`TestWorkflowPreservesIdentity` has 23 cases across read, fresh run and replay:
authorized, replacement identity during source IO, revocation, cancellation,
source failure alone/with revocation/with replacement, and replacement before
final submission. The last case uses the fifth authentication callback (after
two Project lookups) to switch identity without timing assumptions. Denied calls
return zero content/outcome and false replay, with unchanged command-lookup and
commit counters. Positive controls check original attribution and successful replay.
No real agent, paid API or new test dependency is needed.

From `services/platform`, original implementation plus regression:
`TMPDIR=/home/rluders/.cache/amaru-workflow-validation/tmp GOFLAGS=-p=1 go test -count=1 -run '^TestWorkflowPreservesIdentity$' ./internal/coordinator`
FAIL, exit 1, exactly eight identity-binding/error-precedence cases;
`/tmp/191-workflow-before.log`. Restored candidate immediately afterward.

Candidate checks:

- `TMPDIR=/home/rluders/.cache/amaru-workflow-validation/tmp GOFLAGS=-p=1 go test -race -count=100 -run '^TestWorkflowPreservesIdentity$' ./internal/coordinator`
  from `services/platform`: PASS, exit 0, 23 cases ×100;
  `/tmp/191-workflow-stress.log`.
- Root `TMPDIR=/home/rluders/.cache/amaru-workflow-validation/tmp GOFLAGS=-p=1 make check`:
  PASS, exit 0; 348 TypeScript contract tests, 346 identical Go/TypeScript fixture
  outcomes, 48 SDK tests, Platform/domain/architecture tests, Protobuf checks,
  host/cross-OS vet, PostgreSQL-tag vet and docs; `/tmp/191-workflow-check.log`.
- From `services/platform`,
  `TMPDIR=/home/rluders/.cache/amaru-workflow-validation/tmp GOFLAGS=-p=1 go test -race -count=1 ./internal/coordinator ./internal/attach ./internal/adapter/localauth ./internal/arch`:
  PASS, exit 0, all four packages; `/tmp/191-workflow-race.log`.
- Final root `make docs-check` (same TMPDIR/GOFLAGS) and `git diff --check`:
  PASS, exit 0.
- `trail build`: PASS, exit 0. `trail check --base origin/feat/18-wp-m3-06-public-api-events`:
  exit 1, eight lexical conflicts across four tracked changed files (the new
  untracked evidence record was outside that check). Reviewed individually:
  the guide's command-key match changes no fingerprint or CloneRoot hashing;
  its actor match now refuses replacement instead of retaining attribution;
  its Workflow match retains post-source authorization; its cancellation match
  retains authenticate-before-context-error ordering. The helper and test's two
  clone-binding matches retain the single captured `ProjectWorkflow` and never
  re-read ActiveClone. The final Submit and test's two post-source matches retain
  direct Workflow authorization after source IO. This is an explained nonzero
  result, not a passing Trail semantic check.

Initial sandbox attempts could not write the existing Go cache. Escalated
attempts then hit `disk quota exceeded` linking/compiling under `/tmp`, before
tests ran. A task-specific build-temp directory on `/home` resolved this without
removing existing caches or worktrees. These setup failures are not regression
reproductions; the eight-failure run above is the actual reproduction.

Evidence level: **unit-tested**, fake source/authentication/storage plus actual
coordinator Submit/Policy/replay. No new PostgreSQL runtime, real session/browser,
Provider, product startup/process restart, public Workflow HTTP/SDK or native
non-Linux validation. Full #191/#18/M3 acceptance remains open.
