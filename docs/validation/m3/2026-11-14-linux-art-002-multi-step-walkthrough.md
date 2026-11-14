# ART-002 internal multi-Step Evidence walkthrough — Linux

Date: 2026-11-14. Local Linux validation, not hosted CI.
Branch: `feat/124-multi-step-evidence-walkthrough`, based on #19 integration
head `b9662a89935eb32586281817d476b605edb2f178` (merged PR #391).
Refs #124 and #19; this is a partial slice and neither issue is complete.

## Scope and workspace

Repository instructions, delivery rules, Trail context, all registered worktree
statuses, local/remote branches and GitHub issue/PR state were inspected before
editing. No open PRs existed at selection. Native #124 blockers #119/#123 and
inherited #19 blocker #15 were closed. The current designated PR base remains
`feat/19-wp-m3-07-worktrees-configuration-scopes`; the separate #18 queue is not
this change's base.

Reused `/tmp/amaru-124-api` after verifying its prior #391 had merged and it had
no tracked changes. Its untracked `.trail/.gitignore` is preserved and excluded
from this change. No new worktree was created. Root checkout and other shared
worktrees were inspected read-only; their existing changes were preserved.

## Demonstrated behavior

`TestMultiStepEvidenceWalkthrough` uses the existing coordinator test harness,
in-process Worker link, fake Execution/context/Skill/Artifact/blob stores, real
Artifact Ingester over temporary files, and internal Evidence Reader. It runs
two Steps in a test-controlled sequence, not a Workflow dependency scheduler:

1. Submit an allowed `code.test` Step and an Approval-gated `git.push` Step.
   Assemble context with a pinned Organization Skill for the first Step.
   The initial bundle retains missing Usage and absent second-Step context.
2. Accept a fenced fake Worker success result with reported zero input tokens,
   explicitly unknown output tokens and a precise estimated USD cost. Ingest
   two fixture Artifacts explicitly, in reverse Artifact-id order, through the
   existing Ingester; the bundle groups and orders their versions and preserves
   Platform-computed content hashes, sizes and producing attempt identities.
3. Verify the Worker receives no assignment for the waiting gated Step. Approve
   its recorded Evidence digest as a human Maintainer, then deliver a failed
   fake Worker result with no Usage reports. The bundle retains the failure and
   an accepted empty capture, with a distinct unknown-Usage gap.
4. Verify both Steps have ended while the Execution remains running and
   `Final` remains false on this base. Explicitly cancel the Execution through
   the existing command to obtain `cancelled`/`Final=true`; Step success/failure
   remains intact. No terminal success/failure state is synthesized.
5. Read one bundle and assert Step order, input hashes, context/Skill identity,
   Usage Project/Environment/Execution/Step/attempt/fence/time attribution,
   Artifact links, the Approval decision and human attribution, ordered
   Approval Audit and exact ordered Execution Event history. Replay the first
   result and require duplicate acknowledgement and an identical bundle/digest.
   A missing Execution returns `ErrNotFound` with no partial bundle.

The final bundle explicitly retains `check`, `diff`, `log`,
`workspace_snapshot` and `usage` gaps with nonempty explanations. No recorder,
production behavior, public API, schema, dependency or infrastructure changed.

## Validation

- From `services/platform`: `GOCACHE=/tmp/amaru-124-api-cache go test -count=1 -run '^TestMultiStepEvidenceWalkthrough$' -v ./internal/coordinator`
  passed (exit 0), one top-level test, no skips.
- From `services/platform`: `GOCACHE=/tmp/amaru-124-api-cache go test -race -count=1 -run '^TestMultiStepEvidenceWalkthrough$' -v ./internal/coordinator`
  passed (exit 0), one top-level test, no skips.
- From repository root: `GOCACHE=/tmp/amaru-124-api-cache make check`
  passed (exit 0), using existing dependency/child-process/loopback access.
  Contract parity: 336 fixtures with identical Go/TypeScript outcomes;
  TypeScript contracts: 338 passed, zero failed/skipped; SDK: 17 passed,
  zero failed/skipped. All Go package suites passed; cross-OS vet and pg-tagged
  vet passed. PostgreSQL runtime tests are not included in this target.
- From repository root: `GOCACHE=/tmp/amaru-124-api-cache make docs-check`
  passed after the documentation edits (exit 0, `doccheck: ok`).
- `git diff --check`, `trail build` and
  `trail check --base b9662a89935eb32586281817d476b605edb2f178` passed
  (exit 0). The final staged-file check reported no conflicts and two lexical
  advisories: an unrelated GitHub native-field learning and a personal
  contracts-test sandbox learning. Both were reviewed; the native-field change
  is absent here and the full check passed with the needed execution access.
- One accidental focused-test invocation from the repository root failed
  before running tests (`go.mod file not found`); the module-scoped run above
  passed. An initial formatting command used module-relative paths incorrectly;
  formatting was then applied successfully from the repository root.

Environment: Linux `7.2.11-100.fc43.x86_64`, x86_64; Go `go1.26.0 linux/amd64`,
Node `v25.7.0`, pnpm `11.5.0`, buf `1.69.0`. Run by Codex in the existing local
Linux development environment; machine kind was not independently verified.

## Evidence limits and remaining work

Evidence level: **integration-tested**, with fake Worker messages and fake
stores. No real Provider/agent, PostgreSQL runtime or transaction snapshot,
production Worker, public API/authorization, native macOS/Windows, retention,
expiry or blob availability is validated by this slice. Existing PostgreSQL
read-scope evidence remains separate. No full #124/#19 acceptance is claimed.

Normal Execution success/failure aggregation is absent from this #19 base.
Worker Results have no output field, so fixture ingest is a manual adapter
call, after result acceptance, without an atomic completion guarantee.
The gated Step's context remains absent: adding it after requesting Approval
would change its Evidence and require unsupported re-request behavior. These
limitations are demonstrated or recorded, not repaired with new semantics.
There is no pinned Workflow/Agent snapshot in this fixture, no retry engine,
and no check/diff/log/workspace recorder. The internal Reader's gap list does
not independently describe every absent context or definition; the absent
fields stay visible and are explained here. Usage publication intents are not
public Event delivery. `Final` is terminal state, not completeness or immutable
Artifact availability. Sequential fake-store reads do not prove a shared
snapshot or concurrent-read consistency.

Further recorder or Approval lifecycle semantics require their own reviewed
slice under ADR-0069; any significant new choice needs human approval. This
change makes none. Stop after opening the PR for orchestrator review; no merge,
issue closure, acceptance-checkbox or Project-board mutation is part of this
slice.
