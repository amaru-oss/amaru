# PRJ-002: settings write response authority

## Selection and scope

Selected #191 from #18 integration `990aed50a4f2d34172b7d735c2d8f955b917b704`
on 2026-11-15. GitHub reports native blocker #120 and inherited parent blocker
#15 closed, and no open PRs. PR #416 is merged. The shared checkout contains
uncommitted user work, preserved by using an isolated worktree.

Accepted ADR-0062 already requires live application authorization. Settings
reads and Project resource operations retain the original human identity after
IO, but `UpdateSettings` returned the store result directly. This bounded
continuation captures the admitted identity, preserves storage-error precedence,
then rechecks attachment and the same human before returning saved settings.
No contract, fixture, SDK, dependency, infrastructure or decision changes.
ADR-0070/0071 remain proposed and outside this change.

## Reproduction and evidence

**Unit-tested:** `TestProjectSettingsWriteAuthorityDuringIO` covers unchanged
identity, replacement owner, revocation, cancellation, storage failure and
storage failure together with replacement identity. Before the fix, the three
successful-write denial cases returned saved settings without an error. After
the fix, they return zero settings with the authorization/cancellation error.
Successful commits remain stored after response denial; failed writes leave
the original value intact. Existing storage errors still win.

**Integration-tested with fake storage:**
`TestSettingsSessionLogoutDuringWrite` performs authenticated PATCH through the
real HTTP guard and application, then logs out that session after storage
commits but before the store returns. Before the fix, it returned 200 and the
saved preference. After the fix, it returns the redacted 403 without settings;
later requests receive 401. A second live session reads the committed preference
and the other Project's unchanged default. Requests use `httptest`, with actual
session/CSRF/logout handling, rather than a browser or product server.

## Local checks

Linux amd64; installed Go 1.26.0 (module selects Go 1.27), Node v25.7.0,
pnpm 11.5.0. Results are local, not hosted CI.

From `services/platform`:

- `go test -count=1 -run '^TestProjectSettingsWriteAuthorityDuringIO$' ./internal/coordinator`
  — FAIL before fix, three failing subtests out of six.
- `go test -count=1 -run '^TestSettingsSessionLogoutDuringWrite$' ./internal/adapter/httpapi`
  — FAIL before fix: 200 instead of 403.
- `go test -race -count=20 -run 'TestProjectSettings|TestProjectReadAuthorityDuringIO' ./internal/coordinator`
  — PASS, exit 0.
- `go test -race -count=20 -run TestSettings ./internal/adapter/httpapi`
  — PASS, exit 0.

The first sandboxed test attempt could not write the existing Go build cache;
the reproduced failures and passing checks above used approved cache access.

From the repository root:

- `make check` — PASS, exit 0, including 348 TypeScript contract tests,
  346 identical Go/TypeScript fixture outcomes and 48 SDK tests. Log:
  `/tmp/amaru-191-settings-make-check.log`.
- `make docs-check` — PASS, exit 0 after completing the evidence record.
- `git diff --check` and `git diff --cached --check` — PASS, exit 0.
- `trail build` — PASS. `trail check --base 990aed50a4f2d34172b7d735c2d8f955b917b704`
  reports lexical documentation conflicts, not a passing review. The conflicts
  match words about replacement authority; the code explicitly rejects a
  changed ID and its failing-before/passing-after regression demonstrates this.
  Advisories concern other Workflow/lifecycle/clone rules: authentication and
  error ordering are retained, and those components are unchanged.

## Limits

Authorization is not atomic with commit or response delivery. A denied response
does not imply rollback, and an authorized later read is needed to observe the
stored state. The original human is retained within `UpdateSettings`; separate
handler operations do not acquire a shared identity transaction. Real compact
sessions use a stable owner ID; replacement identities are exercised with a
trusted fake authentication callback.

No new PostgreSQL runtime, browser, real Provider, product startup or native
non-Linux validation. Cross-OS vet in `make check` is only an early signal.
Full #191/#18/M3 acceptance remains open. No merge, issue closure or Project
board changes are part of this PR.
