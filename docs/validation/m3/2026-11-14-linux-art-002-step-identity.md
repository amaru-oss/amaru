# ART-002 ambiguous Step identity rejection — Linux

Date: 2026-11-14. Runner: Codex, local Linux x86_64; machine classification
not independently verified. Local evidence, not hosted CI.
Refs [#124](https://github.com/rluders/amaru/issues/124) and parent
[#19](https://github.com/rluders/amaru/issues/19); both remain incomplete.
Branch: `fix/124-ambiguous-step-evidence`, based on designated integration
`feat/19-wp-m3-07-worktrees-configuration-scopes` at
`e44bd072dcfc4577b54517830422c60387481220`.

## Selection and scope

Consulted Trail, repository policy and delivery guidance, refreshed origin and
GitHub's open M3 queue, native blocked-by relationships, parents and open PRs,
then inspected all registered worktree statuses before choosing work. No open
PRs existed. Native #119/#123 and inherited #15 were closed; their implementation
and accepted ADR-0069 are present on #19. Reused `/tmp/amaru-124-api` on a new
topic branch from that base, preserving its untracked `.trail/.gitignore` and
all other worktrees' changes.

This bounded implementation rejects ambiguous attribution in the existing
[Evidence assembler](../../../services/platform/internal/domain/execution_evidence.go).
Its Step index previously accepted empty and duplicate IDs, and later contexts
silently replaced earlier contexts for the same Step. All five regression cases
returned a successful bundle before the fix. The assembler now returns
`ErrInvalid` and an empty bundle for empty Step IDs, duplicate/conflicting Steps
and duplicate/conflicting contexts. Identical duplicates are invalid too: this
is one assembled read, not a capture replay/deduplication boundary.

Existing [ContextStore](../../../services/platform/internal/ports/context.go)
and PostgreSQL primary keys already define one immutable context per Step and
unique Step identity. This strengthens the shared domain read boundary without
changing capture identities, contracts, schemas, migrations, permissions,
dependencies or infrastructure. Existing Reader and transaction-bound PostgreSQL
composition both reach this assembler. Valid creation order and digest behavior
are covered by existing regression suites. No new architecture decision is made.

Other queues retain their gates: #188 lacks #135 prerequisite code on #16;
#197 waits for #124 and #125 waits for #197. Provider migration decisions
#80–#82 remain open. On #18, Agent authority remains owner-deferred, and
ADR-0070's Work command contract remains proposed. #92/#170/#191/#219 have closed
native prerequisites but incomplete integration acceptance. None of those facts
is changed by this slice; no integration branches are merged here.

## Validation

From `services/platform`:

- Before production edits: `GOCACHE=/tmp/amaru-124-api-cache go test -count=1 -run '^TestBuildExecutionEvidenceRejectsAmbiguousSteps$' -v ./internal/domain`
  failed as expected (exit 1), all five cases accepted invalid inputs. An earlier
  fixture revision let unrelated reference checks reject two cases; the final
  fixtures preserve valid references and reproduce all five failures.
- The same focused command after the fix: PASS (exit 0), one test with five
  subtests, no skips.
- `GOCACHE=/tmp/amaru-124-api-cache go test -race -count=1 ./internal/domain ./internal/evidence ./internal/ports/fakes ./internal/coordinator ./internal/store/postgres`:
  PASS (exit 0), all five packages. The store package here is untagged and does
  not exercise a PostgreSQL server.

From repository root:

- `GOCACHE=/tmp/amaru-124-api-cache make check`: first sandbox run FAIL (exit 2)
  in the Node contract fixture file with only a generic `test failed` diagnostic.
  Rerun with execution access PASS (exit 0), no code changes between attempts.
  The exact sandbox failure cause was not independently established. Final log:
  `/tmp/amaru-124-ambiguous-check-access.log`. Go tests, documentation,
  Protobuf checks, Go/TypeScript fixture parity, SDK generation/type/tests,
  host/cross-OS and PostgreSQL-tagged vet all passed. Cross-vet is not native
  OS or database runtime evidence.
- `GOCACHE=/tmp/amaru-124-api-cache make docs-check` and `git diff --check`:
  PASS (exit 0).
- `trail build` and
  `trail check --base e44bd072dcfc4577b54517830422c60387481220 --json`:
  PASS (exit 0), no conflicts. The lexical advisory about a personal Node/`buf`
  sandbox learning does not concern Step identity logic; the failed sandbox
  check and passing execution-access rerun are recorded above. The Linux-first
  learning is followed. Optional deep graph analysis was not run.

Tool versions: Go `go1.26.0 linux/amd64`, Node `v25.7.0`, pnpm `11.5.0`,
buf `1.69.0`.

### Continuation verification (2026-11-14)

The owner confirmed this validation invariant is an in-scope follow-up partial
slice of #124; no separate child issue is required. The preserved implementation,
five regression cases and earlier validation results above were retained.
After refreshing origin, the designated #19 base still matched `e44bd07`.

- From `services/platform`, the focused command above was rerun: PASS (exit 0),
  all five subtests, no skips. The same affected-package race command was rerun:
  PASS (exit 0), all five packages.
- From the root, `GOCACHE=/tmp/amaru-124-api-cache make check` first failed in
  the sandbox (exit 2), again with only the generic Node contract-fixture
  `test failed` diagnostic. The same command with execution access passed
  (exit 0), with no code changes between attempts. Logs:
  `/tmp/amaru-124-continuation-check-sandbox.log` and
  `/tmp/amaru-124-continuation-check-access.log`. This does not establish the
  exact cause of either sandbox failure.
- `GOCACHE=/tmp/amaru-124-api-cache make docs-check` and `git diff --check`:
  PASS (exit 0) after this record update.
- `trail build` and
  `trail check --base e44bd072dcfc4577b54517830422c60387481220 --json`:
  PASS (exit 0), no conflicts; the same lexical Node/`buf` advisory and
  Linux-first finding described above remain. Optional deep analysis was not run.

These are local Linux results, not CI or full ART-002 acceptance. The existing
untracked `.trail/.gitignore` is preserved outside the focused commit. No storage
implementation or storage decision is included.

## Evidence limits

The malformed-input behavior is **unit-tested**. These regressions inject invalid
assembler inputs; they do not reproduce a PostgreSQL producer returning duplicate
rows. Existing database uniqueness constraints are unchanged. No PostgreSQL
runtime, native macOS/Windows, real Provider, public Evidence API, missing
check/diff/log/workspace recorder or full #124/#19 audit chain is validated by
this slice. No acceptance checkboxes, issue closures, Project status changes or
merges are part of this work. Linux-first M3 policy remains in force.

## Step ownership before secondary reads

`TestBundleRejectsMismatchedStoreIdentity` supplies a foreign Step from the initial Execution lookup. The reader returns no Evidence and performs no secondary reads. The domain builder still checks each Step when called directly.
