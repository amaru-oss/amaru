# OBS-001 Worker process-channel capture — Linux

Date: 2026-11-14. Runner: Codex, local Linux x86_64,
`7.2.11-100.fc43.x86_64`; `systemd-detect-virt` reports `container-other`.
Physical/full-VM status is unverified. Local results are not hosted CI.
Base: #18 integration `feat/18-wp-m3-06-public-api-events` at
`2ea3069bbbec07ec31ffeba6ffb4c26b6a220316` (merged #395).
Source: this record's commit, branch `feat/170-composed-output-capture`.
New isolated worktree: `/tmp/amaru-170-output`. Existing branches, worktrees
and untracked data, including `/tmp/amaru-124-api/.trail/.gitignore`, preserved.

## Live selection and scope

Refreshed GitHub issues, native direct/inherited blockers, open PRs and remote
integration heads before selecting work. No open PRs were observed. #170's native
#83/#84 and inherited #18 prerequisite #15 are closed. #92 shares these satisfied
prerequisites; canonical startup/state directory and CLI composition remain open.
#170 was selected for its explicit verification requirement to capture Provider
failure and cancellation with secret fixtures across output channels. Its existing
Worker instrumentation tests use a buffer and a manually driven protocol peer;
HTTP/Lifecycle capture does not exercise the execution owners with the Coordinator.
This slice joins those existing components under accepted behavior, with no new
architecture choice, API, dependency or infrastructure.

| Queue | Current disposition |
| --- | --- |
| #170 / #92 / #191 / #219 | Native prerequisites closed; remaining #18 integration acceptance open. Agent authority remains deferred and Work contract ADR-0070 remains proposed. |
| #124 / #154 / #188 | Native prerequisites closed; their bounded #19/#16 slices remain distinct from #18. |
| #80 / #81 / #82 | Provider migration decisions remain human gates; no migration performed here. |
| #112 / #113 / #127 / #128 / #131 / #163 | Blocked by open #80/#81/#82. |
| #189 / #190 | Blocked by #113, and #190 also by #127. |
| #197 / #125 | Blocked by #124 and #197 respectively. |
| #16 / #17 / #18 / #19 / #20 | Parent integration and exit acceptance remain open. |

PR #384 (Usage), #394 (walkthrough) and #397 (check-result assessment) were
verified merged into #19. None is duplicated; no check-result schema assessment
was repeated. Observed #19 head: `1e5c9e6d93fa1e0acdc1c0b0c99d84e3d5f8320e`.
Those changes are not assumed present on this #18 base. No issue checkbox, issue
closure, merge, board mutation or M3 exit claim is made.

## What the test establishes

`TestExecutionDiagnosticChannels` launches the test binary as a child for success,
Provider failure, panic and explicit Coordinator cancellation. Each child runs
actual Lifecycle, Session, EmbeddedWorker, Coordinator and in-process transport
with fake Execution storage and a fake Runner wrapped by `ObserveRunner`.
The recovered Coordinator is test-supplied after real fake-store reconciliation;
this does not exercise PostgreSQL Recovery initialization.

The child waits for the expected committed Execution and Step terminal states,
then stops and joins the lifecycle owners before closing/draining the bounded
Runner diagnostic queue. Cancellation uses the real `Coordinator.Cancel` path,
not process shutdown. Panic uses the existing Worker panic boundary. The consumer
checks attempt identity against the committed Step and obtains Project identity
from the Execution, then writes through the existing JSON logger to stderr.

The parent captures stdout and stderr separately. Stdout must be empty; stderr
must contain exactly two JSON records, with the exact existing field set, valid
timestamps, trusted Instance/Project/Execution/Step identity and matching nonempty
attempt correlation. Outcomes must be `started` followed by the selected safe
outcome. Opaque, unregistered secret fixtures occur in inputs, failure detail,
panic payload and cancel reason; neither captured channel may contain the fixture.
Assertions withhold captured payloads on failure. The child exits only after joins,
suppressing the test harness's ordinary PASS output. The helper test skips in a
normal parent run and executes in each of the four children.

Scope reaches one new test file, this record and links in the Platform README and
validation guide. No production callers, contracts, exports, configuration or
fixtures change. The test-owned consumer is deliberately explicit; a production
entrypoint/consumer does not yet exist. Raw fake Provider fields may remain in
domain state under existing semantics; this test proves their omission from these
operational process channels, not redaction of every persistence/read surface.

## Commands and evidence

Tool versions: root Go launcher 1.26.0; Platform module Go 1.27.0;
Node v25.7.0; pnpm 11.5.0.

- Initial focused command used a root-relative edit path from `services/platform`
  and failed before the edit; the accompanying test attempt failed on the default
  read-only Go cache. Corrected directory and temporary cache before running tests.
- From `services/platform`,
  `GOCACHE=/tmp/amaru-170-output-cache go test -race -count=1 -v ./internal/platform -run '^TestExecutionDiagnostic'`:
  PASS, exit 0, all four scenarios; helper skipped in parent and executed in children.
- `GOCACHE=/tmp/amaru-170-output-cache go test -race -count=10 ./internal/platform -run '^TestExecutionDiagnosticChannels$'`:
  PASS, exit 0, 40 child executions, 42.380s.
- Initial root `GOCACHE=/tmp/amaru-170-output-cache make check`: FAIL, exit 2,
  spike cross-vet compilation hit disk quota. Existing data/caches were not deleted;
  rerun uses the existing default cache.
- Root `make check` using the existing default cache: PASS, exit 0.
  Final `make check` after test/doc edits: PASS, exit 0, including 348 TypeScript
  contract tests, 346 identical Go/TypeScript fixture outcomes, 48 SDK tests,
  Platform tests, docs-check, Protobuf checks and host/cross-OS/pg-tag vet.
  Cross-vet does not establish native macOS/Windows behavior.
- From `services/platform`,
  `GOCACHE=/tmp/amaru-170-output-cache go test -race -count=1 -v ./internal/platform ./internal/adapter/operational`:
  PASS, exit 0, 48 top-level tests passed; one helper skipped in the parent
  and exercised in each child.
- Final root `make docs-check` and `git diff --check`: PASS, exit 0.
- `trail check --base origin/feat/18-wp-m3-06-public-api-events`: exit 1,
  three lexical conflicts in the new README/validation-guide prose. Two match
  the session-link cancellation learning: this change does not close the link
  on Start cancellation; it uses the existing reverse Stop and drains only
  after Lifecycle returns. The third matches the SQL driver panic learning:
  this test panics a fake Runner inside the existing Worker boundary, not a
  database driver; no driver cleanup or PostgreSQL evidence is claimed.
  Other lexical advisories were reviewed; no production behavior changes.
- `make platform-test-pg`: not run; `AMARU_TEST_PG` is unset in this session.
  PostgreSQL-tag vet in `make check` is compile evidence only.

Logs: `/tmp/amaru-170-output-check.log`,
`/tmp/amaru-170-output-check-default-cache.log`,
`/tmp/amaru-170-output-repeat.log`,
`/tmp/amaru-170-output-check-final.log`, `/tmp/amaru-170-output-race.log`,
`/tmp/amaru-170-output-trail-staged.log`.

## Limits and #20 exit checklist

The new channel capture is **integration-tested** with actual Amaru execution
owners and a fake Provider/store. It is not real Provider validation or production
entrypoint acceptance. Full #170 command/Worker/output composition remains open.
Startup/CLI, HTTP/Event assembly, runtime secret registration/rotation, process
restart/crash and PostgreSQL persistence are not newly validated here. Native
macOS/Windows remain **not validated**, best effort for M3.

#20 still requires same-candidate Linux behavior/contracts, crash injection,
ungranted Capability refusal, restart-safe Approvals, Codex/OpenCode confinement
refusal, the core identifier audit and a recorded real Linux headless Provider run.
This PR does not tick any exit box. Default suites are checked locally below;
missing Provider decisions and real-run evidence remain explicit gates.
