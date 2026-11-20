# ART-002 Evidence bundle ownership — Linux

Date: 2026-11-14. Local Linux x86_64, kernel `7.2.11-100.fc43.x86_64`;
results are not hosted CI. Versions: Go launcher 1.26.0, Platform Go 1.27.0,
Node v25.7.0, pnpm 11.5.0. Run by Codex in the owner's Linux environment;
physical/VM classification was not verified.
Issue: #124, parent #19. Branch: `fix/124-evidence-owned-snapshots`.
Isolated worktree: `/tmp/amaru-124-owned-snapshots`, based on #19 integration
`cafe0c674d42e386a05c968b0c92deebb1d620a1`. Source: this record's commit.
Existing worktrees and untracked files were preserved.

## Exact-head review and merge

Reviewed #399 at `d24add5e5a3bc01e6fe036065843ba402a0c3bd9`.
No actionable issue found in adapter locking, replay checks, ingest error
propagation or shared conformance coverage. Authenticated `rluders` is the
author, so formal self-approval is unavailable. The user explicitly authorized
an exact-head COMMENT review and merge if mergeable.

[COMMENT review](https://github.com/rluders/amaru/pull/399#pullrequestreview-5905768260)
records the head and checks. `gh pr merge 399 --merge --match-head-commit
d24add5e5a3bc01e6fe036065843ba402a0c3bd9` succeeded. GitHub readback verified
`MERGED`, merge SHA `cafe0c674d42e386a05c968b0c92deebb1d620a1`, targeting
`feat/19-wp-m3-07-worktrees-configuration-scopes`.
#124, #19 and #20 were verified OPEN afterward.

Exact-head local checks for #399:

- Root `make check`: PASS, exit 0.
- From `services/platform`, `go test -race -count=1 ./internal/ports/fakes ./internal/artifact ./internal/evidence ./internal/coordinator ./internal/store/fsblob ./internal/store/postgres`:
  PASS, exit 0, six packages. Untagged PostgreSQL tests are unit evidence.
- Logs: `/tmp/pr399-review-check.log`, `/tmp/pr399-review-race.log`.

The prior PostgreSQL runtime record was reviewed, not rerun for #399 here.

## Live queue and selection

Refreshed GitHub after merging #399: no open PRs remained. The milestone is
named **M3 Platform core**, not `M3`. Native blockers #119/#123 for #124 and
inherited #15 for #19 are closed; prerequisite code is present on this base.
#197 still waits for open #124; #125 still waits for open #197 (#195 closed).
The #124 integrity path therefore supports the downstream Approval/retention
chain without starting blocked work.

Other candidates remain: #154 has closed #85/#120; #170 has closed #83/#84;
#191 has closed #120; #219 has closed #135 and inherited #15. Their remaining
production composition and acceptance obligations are not completed by this
slice. #188's latest assessment still records missing prerequisite code on
its designated #16 base; closed prerequisites alone do not fix that. Provider
migration #80/#81/#82 remain owner gates. #16/#17/#18/#19 and exit #20 remain
open. No issue closure, acceptance-checkbox or Project status change is made.

Selected a reproducible Evidence integrity defect: `BuildExecutionEvidence`
copied pointers and nested slices from its input. Editing a Usage measurement,
Agent setting, context item, Approval Evidence item or definition snapshot after
assembly changed the returned contents without updating the Digest. Conversely,
editing the bundle changed the supplied source records. Store adapters already
detach their own reads, but that does not give the assembler independent
ownership of the records it is passed.

## Change and scope

The assembler now decodes its already generated, hashed JSON into a fresh typed
bundle before returning it. This gives the bundle ownership of the represented
data, without a second encoding, new helper, dependency, persistence or public
contract. The regression checks both mutation directions and recomputes the
Digest from the returned representation. It covers nested definition bytes,
timestamps, Agent slices, Step errors, context items, Approval items/decision,
Usage reports, integer/credit measurement values and cost.

The existing JSON serialization is reused instead of introducing a manually
maintained clone for every nested field. This adds one JSON decode and its
allocations per bundle; no performance or bounded-total-size claim is made.
Optional empty slices may decode as nil when omitted by existing JSON tags;
their serialized representation is unchanged. Returned Go values remain mutable:
a caller that edits its own bundle must recompute its Digest. Concurrent input
mutation while assembly is running is not supported or synchronized here.

This stays inside the pure domain assembler and its existing Reader callers.
Accepted ADR-0069 read consistency and ADR-0036 Evidence attribution remain
unchanged. Per [ADR-0017](../../architecture/adr/0017-content-addressed-artifact-store-filesystem-or-s3.md),
Git remains Project/source history; ArtifactStore owns immutable output blobs,
and PostgreSQL owns version/provenance metadata. Filesystem remains the compact
adapter and RustFS the reference S3-compatible backend. No Project tree output,
new infrastructure, public endpoint, recorder semantics or ADR acceptance is added.

## Continuation validation

Before the production edit, from `services/platform`,
`go test -count=1 ./internal/domain -run TestExecutionEvidenceOwnsItsHashedContents`:
FAIL, exit 1, both `input` and `bundle` mutation cases. Log:
`/tmp/pr400-ownership-before.log`. A later sandbox attempt could not access the
Go cache; validation uses execution access. One formatting invocation used an
incorrect relative path; the corrected root invocation succeeded.

- From `services/platform`, `go test -race -count=1 ./internal/domain ./internal/evidence ./internal/coordinator ./internal/ports/fakes ./internal/store/postgres`:
  PASS, exit 0, five packages, including both ownership regression cases and
  existing Reader/Coordinator callers. Unit/fake-backed integration evidence;
  PostgreSQL runtime tests were not rerun because no database behavior changed.
  Log: `/tmp/m3-evidence-ownership-race.log`.
- Initial root `make check`: FAIL, exit 2, because Go compilation/linking hit
  `disk quota exceeded`; the Attach Git fixture also failed writing its config.
  TypeScript contracts 338/338, Go/TS parity 336/336 and SDK 17/17 passed before
  the failure. `quota -s` confirmed `/tmp` near its per-user hard limit, despite
  filesystem-wide free space. No test failure is waived and no old files deleted.
  Log: `/tmp/m3-evidence-ownership-check.log`.

- Root `GOFLAGS=-p=1 make check`: PASS, exit 0. The same full validation target
  runs with serial package builds to reduce peak temporary storage. Contracts
  TypeScript 338/338, Go/TS parity 336 identical outcomes, SDK 17/17, all
  Platform tests, host/cross-OS/pg-tag vet, Protobuf and documentation checks
  passed. Cross-OS vet is not native OS evidence.
  Log: `/tmp/m3-evidence-ownership-check-serial.log`.
- `gofmt -w services/platform/internal/domain/execution_evidence_test.go` and
  `git diff --check`: PASS. Final `make docs-check`: PASS, exit 0 (`doccheck: ok`).
- Optional graph diff check was not performed: installed `graft check` rejected
  `--base` as an unknown option. Source callers were inspected with the graph
  and a repository search; this is not a claim that the optional check passed.

## Remaining acceptance

This is a bounded ownership fix, not a sealed bundle, cross-store read guarantee
or public Evidence acceptance. Authorized versioned query composition, blob
availability, remaining checks/diff/log/workspace producers and the complete
audit chain stay open. Multi-output metadata writes remain non-atomic, and
rejected Artifact ingest can leave unreferenced blobs. #124/#19/#20 remain open.
Real Providers, RustFS, native macOS/Windows and complete M3 exit are not validated.

## Execution Event sequence completeness

The per-Execution store returns the full log, whose sequence starts at 1 and
has no gaps. Evidence assembly refuses a missing first or intermediate
sequence with ErrInvalid and returns no bundle. Both missing-entry cases
returned a digest before the change. Approval audit sequence rules are
independent because those entries are selected from the Instance audit log.
