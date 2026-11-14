# Settings HTTP session loss during storage IO — Linux

Date: 2026-11-14. Runner: Codex for Ricardo Lüders. Local Linux x86_64,
kernel `7.2.11-100.fc43.x86_64`; physical/VM qualification not verified.
Go launcher 1.26.0, Platform toolchain 1.27.0, Node v25.7.0, pnpm 11.5.0.
This record covers the test/documentation slice based on
`361f583ad79f1578d9d0171a34b172e59c89a9e2`; the delivery PR identifies its
exact committed head. All results are local, not hosted CI.

## PR #402 exact-head review and merge

The user reported no actionable diff finding and authorized verification,
COMMENT review and merge. PR [#402](https://github.com/rluders/amaru/pull/402)
and `/tmp/amaru-191-read-authority` both matched
`362c2f0fff6f2080b5044519e0b7eb0c55ba9c8e`. Tracked work was clean; untracked
`.trail/cache/` was preserved. Runtime changes, tests and evidence were inspected;
no actionable issue found. GitHub reported MERGEABLE/CLEAN, zero check runs
and zero legacy statuses; the repository requires local checks.

Fresh exact-head `GOFLAGS=-p=1 make check` passed, exit 0: 348 contract tests,
346 identical Go/TypeScript fixture outcomes, 48 SDK tests and all required
Platform, vet, Protobuf and docs checks. Log:
`/tmp/pr402-review-check-unsandboxed.log`. From `services/platform`,
`go test -race -count=1 ./internal/coordinator ./internal/adapter/httpapi ./internal/adapter/localauth`
passed all three packages, exit 0; log `/tmp/pr402-review-race-unsandboxed.log`.
`git diff --check 39386f7e9cd2782075d1cf865955a39ccef71a26...HEAD` passed.
Initial sandbox runs failed on Node fixture subprocesses and loopback binding;
unchanged checks passed outside the sandbox.

The [COMMENT review](https://github.com/rluders/amaru/pull/402#pullrequestreview-5905943493)
is pinned to the requested head. Authenticated author `rluders` cannot
self-approve. `gh pr merge 402 --merge --match-head-commit 362c2f0fff6f2080b5044519e0b7eb0c55ba9c8e`
succeeded. GitHub readback verified MERGED at
`361f583ad79f1578d9d0171a34b172e59c89a9e2`. Fetch and local parent inspection
confirmed that commit has the reviewed head as its second parent and an
identical tree. No issue was closed.

## M3 queue refresh and selection

Refreshed all 25 open issues in `M3 Platform core`, their native blocked-by
relationships, candidate issue bodies, open PRs and remote refs. No competing
open PR remained after #402. #191's native #120 and inherited #18 prerequisite
#15 are closed; #83/#84 are closed and their accepted boundaries are on #18.
The #15 integration commit is an ancestor of the selected base.

- #191, #92 and #170 can advance bounded #18 composition/evidence slices;
  their full product acceptance remains open.
- #219's #135 is closed and on #18, but authoritative mutations and external
  definition reconciliation remain separate work. Work mapping still needs the
  proposed ADR-0070 decision.
- #124 can advance on #19, whose remote head is
  `1de18c0265608e0d77738edbf2f9e5588f20b523`. Public Evidence mapping remains
  proposed in ADR-0071; this slice does not implement it. #197 is blocked by
  open #124; #125 is blocked by open #197.
- #188's native #135 is closed, but the designated #16 base remains
  `cd5c17d740315bb471f735b60257df2cab4bddbe`, without the ordered Event port.
  Closure alone does not make its implementation ready on that base.
- #80/#81/#82 remain open migration decisions with `awaiting-owner-approval`;
  their Provider, hermetic-run, discovery and secret-backend dependents remain
  blocked. #189/#190 retain open native prerequisites. #154's closed native
  prerequisites do not establish production Worker composition.
- #16/#17/#18/#19 and exit #20 remain open. Child merges are not parent or
  milestone acceptance; no acceptance checkbox or Project status was changed.

Selected a small #191 integration regression independent of those decisions:
the prior matrix substituted an authentication callback, while the real HTTP
handler must observe logout from its actual Auth instance after storage IO.
Created `/tmp/amaru-191-settings-session-io`, branch
`feat/191-settings-session-io`, from the verified #402 merge. Existing worktrees
and uncommitted work were preserved.

## Regression and evidence scope

`TestSettingsSessionLogoutDuringRead` covers GET and PATCH. A deterministic
storage hook logs out the requesting browser session using the real logout
handler after obtaining the stored value. The settings response must be a
redacted 403, with no settings payload, body consumption or write. A subsequent
request from the logged-out session is a 401 before storage. A second live
session still reads both same-name Projects, preserving selected true and
other-Project false preferences.

The regression was run with only `project_settings.go` temporarily replaced by
its pre-#402 version, then automatically restored byte-for-byte. Both cases
failed as expected: GET returned 200 with settings; PATCH consumed its body
before rejection. Command from `services/platform`:
`go test -count=1 -run TestSettingsSessionLogoutDuringRead -v ./internal/adapter/httpapi`.
Exit 1, two failures; log `/tmp/191-settings-session-regression.log`.
The restored runtime has no diff. No runtime, schema, SDK, dependency,
infrastructure or authority model change is introduced.

## Validation

- `go test -race -count=1 -run TestSettingsSessionLogoutDuringRead -v ./internal/adapter/httpapi`:
  PASS, exit 0, GET and PATCH; `/tmp/191-settings-session-focused.log`.
- Root `GOFLAGS=-p=1 make check`: PASS, exit 0; 348 contract tests,
  346 matching Go/TypeScript fixture outcomes, 48 SDK tests and all required
  Platform, architecture, vet, Protobuf and docs checks.
  Log: `/tmp/191-settings-session-check.log`.
- From `services/platform`, `go test -race -count=1 ./internal/adapter/httpapi ./internal/adapter/localauth`:
  PASS, exit 0, both packages; `/tmp/191-settings-session-race.log`.
- Final documentation and whitespace checks are recorded in the delivery PR.

Evidence level: **integration-tested**, using real authentication/session and
HTTP handler components, deterministic fake storage and HTTP recorders. No
real browser, serving product process, PostgreSQL runtime, native macOS/Windows
or real Provider validation is claimed. The final authorization check remains
non-atomic with response delivery. No mutation transaction authorization or
full #191/#18/M3 acceptance is established; those issues remain open.
