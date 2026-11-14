# Linux ART-002 timeout Usage verification — 2026-11-14

## Scope and base

Partial #124 continuation, parent #19, M3 exit #20. Worktree
`/tmp/amaru-124-timeout`, branch `feat/124-timeout-usage-capture`, based exactly
on #19 integration `611482781a18987f28d5ec1bee985959622c2455` (#406 merged).
This is acceptance evidence for existing timeout behavior, not a runtime fix.
No new dependency, contract, migration, recorder or storage change.

The shared `coordtest.UsageSuite` previously exercised success, failure and
cancellation, omitting the distinct Worker `timeout` completion path. It now
runs 41 cases against each store: the existing 30 cases, ten Worker-reported
timeout cases and one Platform-settled timeout case. The suite asserts the
stored completion state/Event for every accepted result. Timeout Evidence
retains the timeout error code and redacted message.

Worker-reported timeouts retain single/multiple reports, ordinal identities,
decimal precision and explicit unknowns. Before/after-commit fault injection,
concurrent results, exact/conflicting/reordered replay and unchanged Evidence
digests reuse the existing suite. PostgreSQL checks persisted publication
intents before each accepted/duplicate acknowledgement. The added
Platform-settled case commits an existing timeout transition without a Worker
receipt, then verifies that a late Worker result is rejected as stale: no
receipt/enrichment, unchanged Evidence digest and an explicit Usage gap. This
constructs the settled state through the store; it does not test deadline expiry
or process termination. Existing timeout suites cover their own named cases.

## Orchestration refresh

- `/tmp/amaru-124-usage`: head `45ffd13d703d727fbf2aacafc9d3b1353196e751`,
  #384 merged into #19 at `39ee71196109fbfb62d4775dd18a99fb69799003`;
  original diff 24 files, 932 insertions / 41 deletions.
- `/tmp/amaru-124-next`: head `90b616cdfc205a60482fc2cb31c907f1d4bcb12e`,
  #386 merged into #19 at `5bf1d3d343b82dca173241b89152400f8b4da95d`;
  original diff four files, 141 insertions / eight deletions.
- Both heads are ancestors of the selected base; neither has tracked edits.
  Their untracked `.trail/.gitignore` files and all other worktrees are preserved.
  Related #385/#387/#391/#394/#397/#399–#406 are merged; no open PR existed
  at selection. No original Usage implementation was duplicated.
- Refreshed all 25 open M3 issues and native dependency edges. #124's #119/#123
  and inherited #19 blocker #15 are closed. #197 remains blocked by #124;
  #125 by #197. Parent #16/#17/#18/#19 and exit #20 remain open.
  #80/#81/#82 owner migration decisions still gate #112/#113/#127/#128/#131/#163;
  #189 waits for #113 and #190 for #127/#113. Dependency-ready sibling work
  includes #92/#154/#170/#188/#191/#219, subject to inherited/base prerequisites
  and their unmet acceptance; ready dependencies alone do not prove completion.
- Refreshed remote heads: main `d01934ea51a2839d3f957d23c9814a598a056cba`,
  #16 `cd5c17d740315bb471f735b60257df2cab4bddbe`, #18
  `f732a09c7756f042b6843e570ab5c36d0ada1fa9`, #19 as above. #17 has no
  remaining remote integration ref. Public Evidence ADR-0071 remains proposed;
  #18 authority/API composition is not assumed present on #19.

## Local Linux evidence

Go 1.26.0 linux/amd64; PostgreSQL 17.5; Node 25.7.0; pnpm 11.5.0.
Commands run from `services/platform` unless noted. These are local results,
not hosted CI; no pre-fix failure is claimed for this test-only slice.

- `GOCACHE=/tmp/amaru-124-timeout-cache go test -count=1 -run '^TestUsageCapture$' ./internal/coordinator`: PASS, 41 shared cases.
- `GOCACHE=/tmp/amaru-124-timeout-cache go test -race -count=1 ./internal/domain ./internal/evidence ./internal/ports/fakes ./internal/coordinator`: PASS, all four packages.
- `AMARU_TEST_PG=postgres://usage_test@127.0.0.1:55486/postgres GOCACHE=/tmp/amaru-124-timeout-cache go test -race -count=1 -v -tags pg ./internal/store/postgres -run '^TestUsage(Capture|PublicationAtomicity)$'`: PASS, two top-level tests, 41 shared capture cases, no skips.

- From the repository root: `AMARU_TEST_PG=postgres://usage_test@127.0.0.1:55486/postgres GOFLAGS=-p=1 GOCACHE=/tmp/amaru-124-timeout-cache make platform-test-pg`: PASS, full tagged regression, exit 0.
- `/tmp/amaru-335-postgres/bin/pg_ctl -D /tmp/amaru-124-timeout-pgdata -m fast -w stop`: PASS, exit 0. Subsequent `pg_ctl ... status` returned expected exit 3, no server running.
- Initial full check stopped at docs-check because the separately unstaged Q70 assessment linked an incorrectly named ADR-0043 file. Only that link was corrected under explicit owner instruction; Q70 remains unstaged and excluded from this PR.

A second full-check attempt using the fresh `/tmp/amaru-124-timeout-cache` failed during spike cross-platform vet with `disk quota exceeded`. The required full check was then retried using the existing default Go cache. Neither failed attempt is represented as a successful check.

- From the repository root, using the existing default cache: `GOFLAGS=-p=1 make check`: PASS, exit 0; 338 TypeScript contract tests, 336 identical Go/TypeScript fixture outcomes, 17 SDK tests, required Go/vet/Protobuf/docs targets.
- Final `GOFLAGS=-p=1 make docs-check` and `git diff --check`: PASS, exit 0.

Unit-tested and integration-tested using fake Worker messages and real
PostgreSQL metadata. Context/Artifact stores in the Usage Reader cases are
empty fakes. Real Providers, public Event delivery/API authorization, native
macOS/Windows and full ART-002/M3 acceptance are **not validated**. Publication
intents are not delivered public Events. Ordinary multi-store Reader composition
is not the transaction-aware PostgreSQL Evidence reader. The timeout checks do
not prove whole-Execution automatic completion. No RustFS storage change.

The separately requested Q70 authored-asset assessment is an uncommitted,
unstaged change preserved in this worktree at the owner's direction. It is
excluded from the Usage commit/PR and needs its own future review/context.
Keep #124/#19/#20 open; no acceptance checkboxes are changed.
