# ART-002 public read contract proposal — Linux

Date: 2026-11-14. Runner: Codex for the product owner. Local Linux x86_64,
kernel `7.2.11-100.fc43.x86_64`; physical/VM qualification not verified.
Tools: Go launcher 1.26.0, Platform Go 1.27.0, Node v25.7.0, pnpm 11.5.0.
These are local checks, not hosted CI or a real Provider validation run.

## Exact-head #400 review and merge

Reviewed PR #400 at `f520e05cf28a244f8cfeaa0edd9c18767c2157ce`, in
`/tmp/amaru-124-owned-snapshots`. Tracked audit was clean; untracked
`.trail/cache/` was preserved. Inspected the diff, nested mutation regression,
typed JSON representation, internal Reader and PostgreSQL caller scope, and
the prior validation/limitations. No actionable issue found. The response is
reconstructed into a fresh value; it has no interface-valued numeric fields
that would turn Usage integers into floating point during decoding.

Authenticated account `rluders` is the author, so formal self-approval is
unavailable. [Exact-head COMMENT review](https://github.com/rluders/amaru/pull/400#pullrequestreview-5905828288)
was recorded. Authorized `gh pr merge 400 --merge --match-head-commit
f520e05cf28a244f8cfeaa0edd9c18767c2157ce` succeeded. GitHub readback verified
`MERGED`, merge SHA `5cf51aa0eec42bbbb3b4e59ec38d4fcacf3ff453`, targeting
`feat/19-wp-m3-07-worktrees-configuration-scopes`.

Checks on that exact head:

- `GOFLAGS=-p=1 make check`: PASS, exit 0. Full target with serial Go package
  builds to reduce temporary-storage pressure; no tests skipped by this setting.
- From `services/platform`, `go test -race -count=1 ./internal/domain ./internal/evidence ./internal/coordinator ./internal/ports/fakes ./internal/store/postgres`:
  PASS, exit 0, all five packages. PostgreSQL tests here are untagged unit tests.
- Logs: `/tmp/pr400-review-check.log`, `/tmp/pr400-review-race.log`.

PostgreSQL runtime, real Providers/RustFS and native macOS/Windows were not rerun.

## Live queue and selected continuation

After #400 merged, `gh pr list --state open` returned no PRs. #124 native
#119/#123 and parent #19's blocker #15 are closed, and prerequisite code is
present on the fetched #19 base. #197 remains blocked by #124; #125 by #197
(#195 is closed). #124/#19/#20 were verified OPEN after the merge.

#188's native blocker #135 and parent #16's blocker #15 are closed, but its reviewed
#135 implementation `fa8bb475979dc39e81d05fffd00dfa40eeb87ae7` is absent from
#16 and `main`. Current #16 head is `cd5c17d740315bb471f735b60257df2cab4bddbe`;
`main` is `d01934ea51a2839d3f957d23c9814a598a056cba`. Reading the prior
#396 assessment confirmed that merging `main` alone cannot remove this gap.
No cross-parent prerequisite merge or blocked #188 implementation was started.
#154 still needs real Worker-entry composition, not another helper test.
#170/#191/#219 retain their scoped #18 composition/contract/producer work.
Provider migration #80/#81/#82 remain human gates.

The highest-impact selected #124 slice is its missing public read decision,
required by accepted ADR-0069. A versioned scoped contract is necessary before
implementation and downstream acceptance; repeated internal hardening does not
define it. This is a proposal deliverable, not a claim that API work is ready
to ship or that remaining recorders exist.

Created isolated worktree `/tmp/amaru-124-read-contract`, branch
`docs/124-evidence-read-contract`, from verified #400 merge `5cf51aa`.
All prior worktrees/untracked files are preserved. Numbering inspected across
fetched refs: ADRs reach 0070 and #18 questions reach Q68. ADR-0071 is proposed,
deciders empty; Q69 remains open. The existing conflicting Q61 headings across
#18/#19 were identified and left intact pending branch integration rather than
silently renumbered and breaking anchors.

## Change and evidence level

[ADR-0071](../../architecture/adr/0071-project-scoped-evidence-read-contract.md)
proposes the route, scoped read/projection, metadata digest separate from
Approval identity and blob availability, completeness vocabulary, errors and
bounded single-query policy. DECISIONS and Q69 index it consistently. No code,
OpenAPI registration, JSON Schema, fixture, SDK or accepted ADR is changed.

Architecture review verdict: **needs human decision** on this public contract.
The proposed route/envelope, 10,000-record/16-MiB limits and error/status choices
are not accepted by ADR-0069. It also requires detailed public-field/redaction
review and a valid shared #18/#19 implementation base. No prototype import,
infrastructure, dependency, storage deployment or new authority is introduced.
ADR-0017 ownership remains intact. No acceptance checkbox or issue is closed.

This is source/documentation review, not implemented behavior. Proposed access,
public representation, completeness, limits and availability are **not validated**.
Implementation would need shared fixtures, scoped HTTP/application tests,
transaction/concurrency/failure evidence, SDK parity and the acceptance matrix
listed in the ADR. Full #124/#19 and M3 exit remain open.

## Proposal validation

- Root `GOFLAGS=-p=1 make check`: PASS, exit 0. TypeScript contracts 338/338,
  Go/TypeScript parity 336 identical fixture outcomes, SDK 17/17, all Platform
  tests and host/cross-OS/pg-tag vet, Protobuf and docs checks passed. Serial
  package builds reduce peak temporary storage without changing test selection.
  Log: `/tmp/m3-evidence-read-contract-check.log`.
- `git diff --check`: PASS; ADR-0071/Q69 references and the proposed status/
  empty deciders were reviewed against the ADR-authoring procedure. Final
  `make docs-check` and `git diff --cached --check`: PASS, exit 0.
- No new behavior tests were added because the change is documentation only.
  Existing tests passing do not validate the proposed contract. No PostgreSQL
  runtime, real Provider/RustFS, native non-Linux or complete M3 acceptance run.

Human decision: approve or amend ADR-0071's public route/projection, state/error
vocabulary and initial single-query limits before implementation. The canonical
ADR-authoring skill requires explicit human approval; no acceptance is inferred
from this continuation request or from merging the earlier ownership fix.
