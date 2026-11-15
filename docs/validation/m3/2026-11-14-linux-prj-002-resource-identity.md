# PRJ-002: retain the human across Project resource operations

## Selection and integration state

On 2026-11-14, after the owner requested autonomous continuation, reviewed exact
PR #415 head `a12b53cf27a6b79fa1df3b00c0986e4131e04b6b` and merged it through
the normal PR path. This was a second pass by the authoring Codex session, not
an independent approval. GitHub reported no hosted checks or requested reviews.
Merged #18 integration head is `d6cef7745e264a5a64d2aa3a3acbd5080df0a2cc`;
`TMPDIR=/home/rluders/amaru-m3-170-tmp make check` passed there (exit 0,
`pr415-merged-make-check.log`). No worktree or branch was removed.

Refreshed #18's queue before selecting this slice: #92/#170/#191/#219 remain
open. #191's native blocker #120, inherited #15 and HTTP/auth prerequisites
#83/#84 are closed; #219's native blocker #135 is closed. PR #415 was the only
open PR before its merge. Proposed ADR-0070/0071 remain human-gated and are not
implemented here. No infrastructure is added.

Selected #191 under accepted ADR-0062: existing Attach/Get/List rechecked that
the current identity was an administrator, but accepted a replacement human ID.
Other resource/query checks already refuse that rebinding. This slice keeps
the original ID through the existing checkpoints and nested clone inspection.

## Implementation and evidence level

**Unit-tested with fake ports:** `TestProjectResourcesPreserveIdentity` replaces
the valid owner at each post-admission check. Twelve cases failed before the
fix. All now return `ErrNotPermitted`, zero/nil results and `created == false`.
Three unchanged-identity controls succeed. Registration is not called after a
pre-write denial; if identity changes after registration, the committed clone
remains available while the response is withheld.

The implementation captures the initial ID in `AttachProject`, `Project` and
`Projects`, passes it to `inspectProject`, and uses a local shared administrator
comparison at their existing checks. Existing authentication errors, cancellation,
store/inspection error precedence, ordering, clone capture, metadata and conflict
handling remain in place. No public signature, schema, fixture or generated SDK
changes. Call-site search found the existing Project HTTP adapter and tests;
`inspectProject` has only the detail/list callers, both updated.

The regression supplies a trusted authentication callback. It does not show a
real compact session changing human ID: the current compact adapter identifies
its local owner consistently. It verifies the application boundary's requirement
for trusted composition callbacks and future identity changes.

## Checks

Local Linux, Go 1.26.0, Node v25.7.0 and pnpm 11.5.0. Logs are retained outside
the checkout in `/home/rluders/amaru-m3-170-tmp`.

From `services/platform`:

- `TMPDIR=/home/rluders/amaru-m3-170-tmp go test -count=1 -run '^TestProjectResourcesPreserveIdentity$' ./internal/coordinator`
  — FAIL before fix, twelve replacement-identity cases;
  `resource-identity-before.log`.
- `TMPDIR=/home/rluders/amaru-m3-170-tmp go test -race -count=20 -run '^TestProject(Resources|Attach)' ./internal/coordinator`
  — PASS, exit 0; `resource-identity-race.log`. Includes existing revocation,
  cancellation, invalid-owner, unavailable-clone and committed-attach cases.

From repository root:

- `TMPDIR=/home/rluders/amaru-m3-170-tmp make check` — PASS, exit 0;
  `resource-identity-make-check.log`.
- `TMPDIR=/home/rluders/amaru-m3-170-tmp make docs-check` — PASS, exit 0;
  `resource-identity-docs-check.log`.
- `git diff --check` and `git diff --cached --check` — PASS, exit 0.
- `trail check --base d6cef7745e264a5a64d2aa3a3acbd5080df0a2cc` — exit 1,
  seven lexical conflicts in documentation and thirteen advisories;
  `resource-identity-trail-check.log`. Reviewed individually as below; this
  is not reported as a passing automated review.

## Review and limits

Trail context was consulted before source inspection, including the saved
original-human mutation rule and transport-error-before-cancellation rule.
Reused the existing resource fake and kept the error/commit ordering. No
continuous session validity or atomic authorization/commit guarantee is added.
No PostgreSQL runtime, browser, real Provider, production bootstrap or native
non-Linux run is claimed; default checks include only cross-OS vet. Full
#191/#18/M3 acceptance remains open.

The seven Trail conflicts are documentation matches: fake clone stores (no
filesystem/path imports changed), cancellation precedence (authentication still
runs first), replacement-owner acceptance (the new cases explicitly deny it),
Workflow post-IO checks (unchanged), clone-root hashing (unchanged), pre-v1
migrations (none added), and replacement-owner acceptance again at the validation
index link. The advisories concern unrelated lifecycle/driver/Project-board
code or limits explicitly recorded here. None requires changing this slice;
the Trail tool permits explaining conflicts in the PR description.

Attempted the owner's requested two-way tmux coordination and asked the
orchestrator to send `/clean` before each new task, separately from its task
prompt. It supplied no acknowledgment; provider retries and later launcher
failures were visible, so autonomous work continued without assigning it work.
All existing worktrees, especially `amaru-m3-next-slice`, are preserved.
