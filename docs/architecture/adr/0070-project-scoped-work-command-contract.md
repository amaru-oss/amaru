---
title: Define Project-scoped Work ownership, keys, reopening and command concurrency
status: accepted
date: 2026-11-14
deciders: Ricardo Lüders
---

# 0070. Define Project-scoped Work ownership, keys, reopening and command concurrency

## Context

**Accepted by the product owner on 2026-11-22.** This decision addresses four contract ambiguities identified in [#191](https://github.com/rluders/amaru/issues/191#issuecomment-6869149642) within [#18](https://github.com/rluders/amaru/issues/18). It supplements, and does not supersede, accepted [ADR-0058](0058-command-idempotency-conflicts-and-retention.md) and [ADR-0062](0062-project-bound-public-operations-and-secret-resolution.md). No runtime or wire change is delivered here.

Evidence inspected at `159f38d70b3bf22b1ebecb72524a93ef93c00f34`:

| Ambiguity | Source evidence | Missing decision |
|---|---|---|
| Omitted owner | [CreateWorkItem](../../../packages/contracts/schemas/CreateWorkItem.json) permits omission; [WorkItem](../../../packages/contracts/schemas/WorkItem.json) requires [WorkOwner](../../../packages/contracts/schemas/WorkOwner.json), whose only kind is `human` and whose label is optional | Whether omission is valid and what stored value it means |
| Display-prefix collisions | [WorkKey](../../../packages/contracts/schemas/WorkKey.json) promises Instance uniqueness but calls the format proposed; [domain Work](../../../services/platform/internal/domain/work.go) formats keys but allocates nothing | Durable prefix allocation across Projects and clones |
| Reopen reason | [DOMAIN_MODEL §5.3](../DOMAIN_MODEL.md#53-work-item) and `WorkItem.SetStatus` require a human and reason for `done → in_progress`; [UpdateWorkItem](../../../packages/contracts/schemas/UpdateWorkItem.json) has only `blockedReason`; [WorkChanged](../../../packages/contracts/schemas/events/WorkChanged.json) rejects extra data fields | How the reason reaches the command and survives in history |
| Replay versus concurrent edit | The four Work create/update schemas have no command keys; resources have no versions. [CommandRecord](../../../services/platform/internal/domain/command.go) and [coordinator replay](../../../services/platform/internal/coordinator/command.go) already distinguish duplicate requests from conflicting reuse | Work wire inputs, normalization, replay order and stale-write handling |

[OpenAPI Work routes](../../../packages/contracts/openapi/openapi.yaml) already select a Project. The domain types carry no Project ownership and dependency methods explicitly delegate cross-Project checks to callers. [Project access inventory](../../engineering/project-access.md) records no authoritative Work application/store implementation. Pure domain tests are evidence of domain rules, not proof of persistence, authorization or public Work service behavior.

## Decision

Accepted 2026-11-22. Field names below are approved future contract changes; they are not yet in the contracts or the Platform.

### 1. Responsibility and Project ownership

On create, omitted `owner` means exactly `{"kind":"human"}` with no label. An explicit identical value is equivalent. On update, omission preserves the owner; `null` is invalid. A label is display text, never a principal ID, authorization claim or automatic assignment to the authenticated caller. Do not invent a human identity from a bearer credential. Agents and Teams cannot be owners. A durable principal-linked responsibility model remains open in Q68.

The Platform stores an immutable Project association on each Work Package and Work Item; an Item must belong to a Package in that same Project. The URL selects authority; clients cannot move ownership with a body field, package substitution or display key. Return the existing redacted `not_found` behavior for unknown or foreign resources after authentication/Project authorization. Lists, package filters, dependency reads, mutation and replay use that same authorized Project. Internal ULIDs remain identity; prefixes are never authorization boundaries. Future resource output should include the immutable `projectId` for unambiguous attribution.

Dependencies use Item ULIDs, may cross Packages within the selected Project, and reject missing/foreign, duplicate, self and cyclic edges. Preserve the current maximum of 20. An omitted update list preserves dependencies; `[]` clears them. Evaluate the complete proposed graph in the authoritative transaction, including combined dependency/status patches. Starting or reopening into `in_progress` requires all dependencies done. Serialize graph edits and start checks within a Project so concurrent individually valid patches cannot jointly create a cycle or race past an unmet dependency. A later regression of a prerequisite does not silently change an already running dependent: record a visible committed change for affected people, per the existing start-only rule. Its exact Event mapping remains open in Q68.

### 2. Durable display allocation

Keep the accepted 2–10 uppercase letter/digit prefix alphabet from DOMAIN_MODEL §5.3 and the existing positive 1–999999999 Item suffix range. Proposed allocation deliberately avoids name-derived collisions: allocate `P` followed by the uppercase base-36 representation of a positive Instance-wide counter, at most nine digits (for example `P1`, `P2`, `PA`). Reserve one prefix lazily when the Project creates its first Item; persist it against the canonical Project identity. Two clones of that Project share the reservation and per-Project Item counter. Different Projects with identical names get different reservations. Renaming a Project or changing its active clone never renames existing keys.

The Platform allocates the Item number, prefix reservation if needed, Item ULID and resulting key with the creation transaction. Enforce unique prefix reservations per Instance, unique full Work keys per Instance and unique Item numbers per Project in storage. Never reuse committed reservations or keys; do not wrap or truncate on exhaustion: reject without a partial creation. Rollback may reuse an uncommitted allocation. Concurrent creates use storage serialization/uniqueness and never return the same key for distinct Items. Clients supply neither prefix nor sequence. This is a proposed allocation policy, not an existing allocator or a change to the canonical repository-based Project key of ADR-0046.

### 3. Explicit reopening history

Add proposed `reopenReason` to `UpdateWorkItem`: 1–1000 Unicode characters after trimming surrounding whitespace. Require it exactly when the stored state is `done` and the requested state is `in_progress`; reject blank, over-limit or unrelated uses. Evaluate this state-dependent rule in the application command, not solely in JSON Schema. Only the authenticated human actor may reopen; a body owner label is no authority. Preserve other existing status/Policy rules and dependency checks.

Keep `blockedReason` separate: it only explains a blocked state and is cleared on leaving it. Pass the reopen reason to domain validation, then persist it in the typed `work.item.changed` Event with the actor, previous/new status and resulting version; it need not become a mutable Item field. The current Event schema cannot carry this history and must change before exposure. Never lose the reason when `SetStatus` clears the transient reason argument outside `blocked`.

### 4. Idempotency and version concurrency

Require proposed body `idempotencyKey` on CreateWorkPackage, UpdateWorkPackage, CreateWorkItem and UpdateWorkItem, following the existing execution command convention. Return proposed positive integer `version` on both Work resources, beginning at 1; require proposed `expectedVersion` on updates. A successful update increments once, including a same-value field assignment; an update with only command metadata and no domain patch is invalid. Return a conflict for a stale expected version, without changing Work or emitting a change Event. Version is a domain revision, separate from command keys, Event cursors and Worker attempts.

Apply ADR-0058's existing `(command name, key)` store namespace, with distinct command names for these four operations. Project is request identity, not an extra key namespace. Fingerprint authenticated actor, selected Project, target/Package, expected version and every consequential normalized field, including reasons, dependencies and any linked Workflow/delivery-loop input. Exclude transport metadata and the key itself. Normalize omitted create owner to the value above, absent optional create text to empty text, absent create dependencies to an empty list, and reopen reason by trimming; preserve all other text. Treat dependency order as significant, matching current domain ordering. In patches, distinguish omission from explicit empty text/list; reject null rather than inventing clearing semantics. Include existing supported fields rather than dropping fields absent from the narrower domain type; unresolved Workflow/delivery-loop mappings remain Q68 implementation gates.

Authenticate and authorize the selected Project and target before looking up/disclosing a recorded outcome. Validate request shape and compute its fingerprint; consult durable replay before checking the current version, transition or dependency state. Same request/key returns the original result and original version even after later edits or restart. Different fingerprint returns conflict without revealing the original outcome. A fresh key with an old version is a stale-write conflict. Concurrent identical deliveries produce one outcome; distinct keys editing the same version allow only one successful commit. Rejected requests leave no successful command record, Work write or Work change Event. Preserve no automatic deduplication expiry in M3; production deletion/Organization scope still requires the ADR-0058 follow-up.

### 5. Atomic Work/outcome/Event boundary

The authoritative Platform transaction must validate ownership, graph, status and expected version against one serialized state, then commit Work rows/version, allocations, original command outcome and typed change Event/outbox entry together, or none. A Project-level write lock or equivalent adapter serialization can protect the graph; an Item-only version check is insufficient for two different Items adding opposite edges. Keep domain validation pure; SQL, locks, clocks and durable allocation belong in application/adapter boundaries. Reuse the existing command/outbox semantics; do not publish an Event first and persist Work later.

A crash before commit leaves no successful outcome or visible Work change. A lost acknowledgement after commit is replayed from the outcome without another allocation/version/Event. A failed Event/outcome write rolls back Work too. Concurrent replay losers roll back and read the winning outcome after authority checks. Publish only committed outbox data; restart or duplicate publication cannot create a new logical Work mutation. Event readers retain existing Project filtering and cursor/replay rules; publication does not promise exactly-once network delivery. Replays return historical outcomes and do not publish another change. No Worker or Provider becomes a Work authority, and no new broker/database/dependency is introduced.

### Examples (proposed behavior, not executable fixtures)

- Same-named Projects A and B create first Items: `P1-1` and `P2-1`; another clone of A creates `P1-2`. A retry of the first create returns its original ULID/key, not `P1-3`.
- Omitted create owner and explicit `{"kind":"human"}` normalize identically; `{"kind":"human","label":"Alex"}` changes the request fingerprint and confers no Alex permissions.
- Human updates done Item version 4 with status `in_progress`, `reopenReason: " Fix missed case "`, expected version 4 and a new key. With satisfied dependencies, commit version 5 and history reason `Fix missed case`. Missing reason, an Agent actor or an unmet prerequisite rejects the whole patch.
- Two clients edit version 5 with different keys: one commits version 6, the other gets conflict. After version 7 exists, replay of the winning request still returns version 6. Changing its expected version to 7 while reusing its key conflicts; a new edit needs a fresh key.
- Concurrent A→B and B→A dependency patches cannot both commit. A dependency in another Project is rejected without exposing that Project's title, key or status.

## Consequences

The four public mutation paths become reviewable without conflating retry safety with concurrent edit protection. Stable prefixes trade mnemonic Project names for simple collision freedom. Per-Project serialization costs throughput but protects graph-wide invariants; revisit with measured contention, preserving semantics. Durable outcomes and history grow storage under ADR-0058. Owner labels still cannot identify accountable principals.

Implementation remains blocked on the unresolved mappings in [Q68](../OPEN_QUESTIONS.md#q68-project-scoped-work-contract-and-unresolved-mappings). Update the unreleased current contracts directly under the pre-v1.0 rule; no compatibility shim, backfill, prototype migration or speculative package belongs in this proposal. Preserve existing data/work. Any eventual deployment/purge policy must be resolved before enabling destructive changes.

Affected surfaces for an approved implementation (mapping only; unchanged here):

| Surface | Required follow-up |
|---|---|
| [API schemas](../../../packages/contracts/schemas) and [OpenAPI](../../../packages/contracts/openapi/openapi.yaml) | Four create/update bodies; WorkPackage/WorkItem version and Project attribution; WorkOwner/WorkKey descriptions; lists inherit resource shapes; document conflict/not_found/validation behavior |
| [Event schemas](../../../packages/contracts/schemas/events) | WorkChanged and Event union/metadata for reason, actor, statuses and version; settle dependency-regression mapping first |
| [API fixtures](../../../packages/contracts/fixtures/api/v1) and [Event fixtures](../../../packages/contracts/fixtures/events/v1) | All affected valid/invalid bodies/resources; missing key/version/reason and owner defaults; Go/TypeScript parity. Stateful replay, ownership and races need application tests, not schema-only examples |
| [SDK](../../../packages/sdk/src) | Regenerate generated/schema.ts; typed client mutation/read/error/Event tests; no separate client-owned state authority |
| [Domain](../../../services/platform/internal/domain/work.go) and [coordinator](../../../services/platform/internal/coordinator) | Reuse status/dependency validation; add explicit Project-authorized Work commands/queries and normalization; retain reopen reason before domain clears it |
| [Ports](../../../services/platform/internal/ports), [fakes](../../../services/platform/internal/ports/fakes), [PostgreSQL](../../../services/platform/internal/store/postgres), [migrations](../../../services/platform/internal/store/migrations) | Implement actual Work ownership/version/graph/allocation and atomic outcome/Event storage; shared adapter conformance and fresh-install validation, no placeholder ports/tables now |
| [HTTP adapter](../../../services/platform/internal/adapter/httpapi) and application composition | Guarded Work routes and drift inventory; same authoritative Instance store/auth; two-client/two-Project HTTP/SDK acceptance and replay after recreation |

Required future checks include sequential/concurrent duplicates, conflicting key reuse across Projects/actors, replay after later version/status changes and authority revocation, opposite-edge cycle races, stale writes, foreign Package/dependency rejection, prefix collisions/clones/exhaustion and crash injection before/after commit and during Event/outcome insertion. Run these against fakes and PostgreSQL plus public SDK composition. None is runtime-validated by this documentation change.

Architecture-review verdict at proposal: **needs human decision** (since given, 2026-11-22). Platform ownership, API-only Clients, UI-agnostic contracts, pure domain and local/server adapter parity are preserved. No new runtime, library or infrastructure dependency, secret access, egress or execution permission is proposed. The primary failure risks are partial persistence, unauthorized replay disclosure and graph write skew; the transaction and authority rules above address them as requirements, not proven implementation. Q68 records the remaining evidence gaps.

## Alternatives considered

- Require owner on every create, or infer an authenticated human: requiring it breaks the existing omission allowance without adding principal identity; inference invents a mapping absent from WorkOwner. Prefer an honest human-kind value and keep identity design open.
- Derive a prefix from Project name, URL or a short hash: name collisions/renames need extra arbitration and truncated hashes still collide. Instance allocation gives a checkable uniqueness rule. Human-selected prefixes would add an unrequested configuration surface.
- Put reopen history in blockedReason, intent or a dedicated new endpoint: the first two confuse current state with audit history; an extra endpoint is unnecessary when the existing update can carry a distinct reason.
- Use only idempotency keys, timestamps or last-write-wins patches: keys stop repeats, not lost updates; timestamps are not reliable domain revisions. Require versions as well.
- Lock only the updated Item or append Events after commit: the first permits graph write skew; the second can lose committed change history. Serialize Project graph decisions and commit the logical Event with Work and outcome.

## Approval

Proposed by Codex for #191. Accepted by Ricardo Lüders on 2026-11-22 in the repository task conversation: “ADR approved.” The approval covers the four Work contract choices and the atomic ownership/dependency boundaries above. It does not resolve the remaining [Q68](../OPEN_QUESTIONS.md#q68-project-scoped-work-contract-and-unresolved-mappings) mappings (principal-linked responsibility, Workflow/external/delivery-loop Work fields, dependency-regression Event mapping), which still gate exposing those parts. It authorizes no issue closure or merge.
