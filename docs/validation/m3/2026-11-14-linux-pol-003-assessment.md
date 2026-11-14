# POL-003 audit continuation assessment (Linux, 2026-11-14)

Status: **blocked implementation; partial assessment only** for [#188](https://github.com/rluders/amaru/issues/188), under [#16](https://github.com/rluders/amaru/issues/16). No authority mutation, Event name, API, schema, dependency or Policy semantics changed. This record does not complete either issue.

## Delivery snapshot

Refreshed GitHub and fetched origin on 2026-11-14:

| Item | Observed state |
|---|---|
| Open PR queue | `gh pr list --state open --limit 100` returned no PRs. |
| #16 designated integration head | `d2c393a8a8d4817d18620630cc212644839ae8d0`; local integration worktree matched origin. |
| #188 native prerequisite | #135 closed; inherited #16 prerequisite #15 closed. |
| #135 implementation | Reviewed PR #322 merged at `fa8bb475979dc39e81d05fffd00dfa40eeb87ae7` on #18. `git merge-base --is-ancestor fa8bb475979dc39e81d05fffd00dfa40eeb87ae7 origin/feat/16-wp-m3-04-policy-grants-and` returned **1**: absent from the required base. |
| #18 integration head | `2ea3069bbbec07ec31ffeba6ffb4c26b6a220316`, including latest merged PR #395. |
| #19 integration head | `e44bd072dcfc4577b54517830422c60387481220`, including latest merged PR #394. |
| #197 | Open; native #124 open and #195 closed. |
| main | `d01934ea51a2839d3f957d23c9814a598a056cba`; updating #16 from main alone would not bring in #135. |

Native dependency endpoints and issue bodies were read for #188/#16/#135. Their existing warning that #135 code is absent from #16 remains accurate. Closed issue state does not satisfy code availability on the dependent base ([delivery rules](../../engineering/delivery.md#4-dependencies)). No cross-parent merge or integration-head mutation was performed.

Only one existing #16 worktree/branch candidate was found: `/tmp/amaru-16-integration`, initially clean at `d2c393a`. Trail inspection generated an untracked `.trail/cache/` there; it was preserved. There was no existing #188 branch candidate. This assessment uses a new isolated worktree `/tmp/amaru-188-assessment`, branch `docs/188-audit-prerequisite-assessment`, from the designated origin #16 head. Other worktrees and shared changes, including the original checkout's untracked `.trail/.gitignore`, were preserved.

## Authority mutation inventory on the required base

[ADR-0036](../../architecture/adr/0036-approvals-bind-to-evidence-and-gate-work.md) requires human, Evidence-specific decisions and historical decisions preserved with their Evidence. #188 additionally requires successful authority changes and their attributable Audit Events to share the persistence transaction. The inventory below distinguishes existing writes from absent semantics; it does not equate the separate AuditLog with the #135 Execution Event stream.

| Required mutation | Existing path and evidence | Remaining gap or limit |
|---|---|---|
| Grant create | [GrantAuditEvent](../../../services/platform/internal/domain/audit.go), [PostgreSQL create](../../../services/platform/internal/store/postgres/grant.go), [fake store](../../../services/platform/internal/ports/fakes/grant.go): records human grantor, target, time and ordered Audit sequence within the write. | Creation supplies no reason in the existing Grant audit mapping. A required reason must not be invented or made a new command requirement in this slice. |
| Grant update/revoke/lapse/expire | Same stores and mapper audit lifecycle transitions. Granted authority is immutable; update changes status/revocation only. Human revocation carries its reason; lapse/expiry carry Platform reasons. | No missing audit append found for these supported lifecycle writes. The focused fake suite covers refused writes and cursor replay, not database persistence failures. |
| Approval request | [ApprovalAuditEvent](../../../services/platform/internal/domain/audit.go), [PostgreSQL insertApproval](../../../services/platform/internal/store/postgres/approval.go), [fake Execution store](../../../services/platform/internal/ports/fakes/execution.go): Policy actor, requester in reason, target/time/sequence, in the Execution creation write. | Existing request auditing already exists; replacing it would duplicate Events. Broader attribution and stream integration remain #188 work. |
| Approval approve/reject | Same mapper/stores write decision and Audit together with the Step change. Decision actor is the human approver; reason is decision rationale. | An approval can have an empty rationale under current semantics; rejection requires one. Do not introduce a new approval requirement or fabricate rationale. |
| Approval void | Same mapper/stores record Platform actor and void reason. Cancellation voids requested Approvals. | Already audited for supported writes. Evidence-change invalidation/re-request and explicit replacement relationships are absent; historical decisions must not be rewritten to simulate them. |
| Approval expire | The domain declares the expired state, but [coordinator Approval implementation](../../../services/platform/internal/coordinator/approval.go) explicitly excludes expiry and has no timer. ApprovalAuditEvent has no expired mapping/event kind. | No successful expiry producer to attach an existing Event to; implementing this would require semantics/Event work excluded from this continuation. |
| Policy change | [WithRules and defaults](../../../services/platform/internal/coordinator/policy.go) configure in-memory rules; [ports/policy.go](../../../services/platform/internal/ports/policy.go) defines Grant persistence, not a Policy mutation store. | No persisted Policy-change command, actor/reason input, transaction boundary or Policy audit kind on this base. Auditing evaluation or construction would invent a mutation. |

[PostgreSQL appendAudit](../../../services/platform/internal/store/postgres/audit.go) updates the sequence counter and inserts the Audit row using the caller's transaction. The fake validates mutations before writing and shares the audit log through `LinkAudit`; it has no fallible persistence append. Existing conflict/invalid-input tests demonstrate refused-write behavior, **not injected persistence failure after a write**. Organization-isolated audit replay is also not demonstrated by this record: AuditLog accepts only cursor/limit, with no Organization authority boundary.

## Architecture review and next action

Verdict: **needs prerequisite integration; no implementation selected**. Keep authority and Audit in the existing Platform store boundary. No dependency or infrastructure is needed for this assessment. A second best-effort writer would violate #188 atomicity; adding missing expiry or Policy semantics would exceed the authorized slice. The simpler option is to retain the current audited paths and first resolve prerequisite availability.

Owner/maintainer next action: bring the reviewed #135 prerequisite onto #16 through the delivery workflow, then reassess one concrete supported mutation and its failure boundary. This assessment neither authorizes nor performs that merge. Before claiming a new slice, tests must force persistence failure and verify both unchanged authority and absence of a spurious Event; also verify the successful attributable change. No such new rollback evidence is claimed here.

Next prerequisite-ready continuation: **#124 ART-002 under #19**, whose native #119/#123 and inherited #15 are closed; #15 implementation `4213a55` is an ancestor of current #19. Continue from #19's current head after assessing what PR #394 already covers; do not repeat its partial multi-Step walkthrough. #124 remains open and is not complete. #197 still waits for it. No unrelated implementation was started during this assessment.

## Local validation

Tested source: #16 head `d2c393a8a8d4817d18620630cc212644839ae8d0`; this branch adds documentation only. Runner: Codex for @rluders. Linux `7.2.11-100.fc43.x86_64`, x86_64; `systemd-detect-virt` reports `container-other` (physical/full-VM qualification unverified). Tools: Go host 1.26.0 with module toolchain selection, Node 25.7.0, pnpm 11.5.0, buf 1.69.0.

| Command | Result |
|---|---|
| In `services/platform`: `go test -count=1 -race -v ./internal/domain ./internal/ports/fakes -run 'Test(GrantAuditEvent\|FakeAuditTrail\|FakeApprovalStore\|GrantStoreConformance)$'` | PASS, exit 0; 4 top-level tests, 20 subtests; no failures/skips. |
| `make check` | Initial sandbox attempt failed (exit 2) at `go vet` because the existing Go cache was read-only. Re-run with cache access: PASS, exit 0, including `doccheck: ok`, 338 TypeScript contract tests (336 fixtures with identical Go/TypeScript outcomes), 17 SDK tests, platform vet and tests; no reported test failures/skips. |
| `make docs-check` and `git diff --check` after recording results | PASS, exit 0 for each; `doccheck: ok`, no whitespace errors. |

The passing `make check` compiled PostgreSQL-tagged tests with `go vet -tags pg`; it did not execute the PostgreSQL runtime suite. The full check log was captured locally at `/tmp/amaru-188-make-check.log`.

Evidence level: **unit-tested** existing Grant/Approval fake behavior only. PostgreSQL persistence-failure rollback, integrated #135/#188 stream behavior, Organization-isolated replay, real Providers and native macOS/Windows are **not validated** in this session. No new tests or runtime code were added because the required implementation prerequisite is absent. Local checks are not hosted CI or full #188/#16 acceptance.
