# ART-002 pinned definition integrity — Linux

Date: 2026-11-14. Runner: Codex for Ricardo Lüders. Local Linux x86_64;
machine kind not independently verified. Local evidence is not hosted CI.
Refs #124, parent #19 and exit #20; all remain open.

## Exact-head review and merge of #404

PR [#404](https://github.com/rluders/amaru/pull/404) and its isolated worktree
`/tmp/amaru-124-context-gaps` matched
`919715c3f2e45700a152c38d772828ff91886b12`. Tracked work was clean;
untracked `.trail/cache/` was preserved. Review covered the diff, assembler,
Reader, shared fake/PostgreSQL assertions and Coordinator walkthrough; there
was no actionable finding. GitHub reported OPEN, non-draft, MERGEABLE/CLEAN,
zero check runs and zero legacy statuses. No hosted CI pass is inferred.

Fresh exact-head `GOFLAGS=-p=1 make check` passed, exit 0: 338 TypeScript
contract tests, 336 identical Go/TypeScript fixture outcomes, 17 SDK tests and
required Go, vet, Protobuf and docs checks. From `services/platform`,
`go test -race -count=1 ./internal/domain ./internal/evidence ./internal/coordinator ./internal/ports/fakes`
passed all four packages, exit 0. `git diff --check HEAD^ HEAD` passed.
Logs: `/tmp/amaru-404-check-full.log`, `/tmp/amaru-404-race.log`. The first
sandbox `make check` stopped in the Node fixture suite; the unchanged full
check passed with normal subprocess/cache access.

The [COMMENT review](https://github.com/rluders/amaru/pull/404#pullrequestreview-5906175536)
is pinned to the exact head; authenticated author `rluders` cannot self-approve.
`gh pr merge 404 --repo rluders/amaru --merge --match-head-commit 919715c3f2e45700a152c38d772828ff91886b12`
succeeded. GitHub readback confirmed MERGED at
`649f265267160a022a804dd3e710c3308ce0697d`. Fetch/local inspection confirmed
the reviewed head is the second parent, the merged tree equals that head and
remote #19 resolves to that merge. No issue was closed.

## Refreshed M3 queue

Read all 25 open `M3 Platform core` issues and their native blocked-by links,
candidate acceptance and inherited parent blockers. Refreshed main and #16/#18/#19
remote integration refs. No competing open PR remained after #404 merged.

- #124 has closed native #119/#123 and inherited #15 prerequisites. Existing
  Reader, context, Artifact and pin code is on designated #19. It blocks #197,
  then #125, so this accepted Evidence slice has downstream prerequisite impact.
- #191/#92/#170/#219 have closed native/inherited prerequisites and code on #18
  (`f732a09c7756f042b6843e570ab5c36d0ada1fa9`). Their full product acceptance
  remains open. Work ADR-0070 and public Evidence ADR-0071 remain proposed.
- #188's native #135 is closed. The #16 ref has advanced to
  `cd5c17d740315bb471f735b60257df2cab4bddbe` through assessment documentation,
  but still lacks the ordered Event prerequisite implementation. A changed ref
  and a closed dependency do not establish readiness of that code boundary.
- #80/#81/#82 remain open owner migration gates. Their dependent Provider work
  and downstream #189/#190 remain blocked. #154's closed prerequisites alone
  do not establish production Worker composition. #16–#20 remain incomplete.

Created branch `fix/124-evidence-definition-integrity` in isolated worktree
`/tmp/amaru-124-definition-integrity` from merged #19
`649f265267160a022a804dd3e710c3308ce0697d`. Existing worktrees and uncommitted
files were preserved. No issue closure, acceptance checkbox or Project-board
mutation is inferred from a bounded child merge.

## Change and boundary review

The existing assembler copied and digested a damaged Workflow pin without
calling its validator. The regression demonstrated successful bundles for a
wrong hash, a mismatched Workflow ID and a matching-hash invalid Workflow.
It failed in all three cases before the runtime fix; the valid pin case passed.
Command from `services/platform`:
`go test -count=1 -run '^TestExecutionEvidenceValidatesPinnedDefinition$' ./internal/domain`
returned exit 1. Log: `/tmp/124-definition-regression.log`.

The assembler now calls `DefinitionPin.Workflow` for a present pin and returns
its error with no partial bundle. Reusing the existing validator checks hash,
Workflow identity and structure without a parallel validation policy. Valid
pins are preserved, and nil remains valid for non-Workflow Executions. The
ownership fixture now uses a valid pin; shared fake/PostgreSQL Reader tests
exercise storage and assembly of a valid pin. A Reader regression checks
`ErrInvalid` propagation with no partial Evidence for damaged stored metadata.

Boundary review: sound for this bounded slice under accepted ADR-0069's exact
definition references. Domain remains pure; no new dependency, infrastructure,
storage write, contract, authorization or Approval semantics. Copying the pin
alone was simpler but silently accepted invalid content. The check adds no
side effect or retry state; failure produces no bundle. It does not authenticate
the producer, recover damaged rows, validate Git revision provenance or assess
all expected Evidence kinds. No new significant architecture decision is taken.

## Validation and limits

Go 1.26.0 linux/amd64, Node v25.7.0, pnpm 11.5.0;
Linux 7.2.11-100.fc43.x86_64.

- From `services/platform`,
  `go test -race -count=1 ./internal/domain ./internal/evidence ./internal/ports/fakes ./internal/coordinator`:
  PASS, exit 0, all four packages. Log: `/tmp/124-definition-race.log`.
- Root `GOFLAGS=-p=1 make check`: PASS, exit 0, including 338 TypeScript
  contract tests, 336 identical Go/TypeScript fixture outcomes, 17 SDK tests,
  Go suites, cross-OS/pg-tagged vet, Protobuf and docs checks.
  Log: `/tmp/124-definition-check.log`.
- Final `make docs-check` and `git diff --check` results are recorded in the PR.

Unit-tested and fake-backed integration-tested. The shared PostgreSQL suite
is compiled by `make check`; a database runtime run is separate and is not
claimed here.

No real Provider, public Evidence API/product server, native macOS/Windows or
full #124/#19/M3 acceptance is validated. Remaining recorders, completeness
classification, authorized public reads and proposed ADR-0071 remain open.
