---
title: Project-scoped Evidence reads use a bounded metadata envelope
status: proposed
date: 2026-11-14
deciders:
---

# 0071. Project-scoped Evidence reads use a bounded metadata envelope

## Context

[ART-002 #124](https://github.com/rluders/amaru/issues/124), under
[WP-M3-07 #19](https://github.com/rluders/amaru/issues/19), still needs a public
authorized query. [Accepted ADR-0069](0069-durable-evidence-capture-and-authorized-bundle-reads.md)
requires a reviewed versioned contract, scoped metadata snapshot and separate
blob availability. Its approval explicitly excludes a new wire contract.
This proposal supplies that next decision boundary; it implements no endpoint.

Verified at #19 head `5cf51aa0eec42bbbb3b4e59ec38d4fcacf3ff453`:

- [The internal Reader](../../../services/platform/internal/evidence/evidence.go)
  returns domain records without authorization. [The PostgreSQL reader](../../../services/platform/internal/store/postgres/evidence.go)
  uses one read-only repeatable-read transaction, bounded to 30 seconds, for
  existing metadata. It does not include authorization or read blob bytes.
- [The domain bundle](../../../services/platform/internal/domain/execution_evidence.go)
  has an independent hashed representation after #400. `Final` means terminal
  Execution state; `Gaps` still names absent producers. Internal `ResultCapture`
  contains numeric measurement sources, fencing tokens and result fingerprints;
  it is not a public DTO.
- [OpenAPI](../../../packages/contracts/openapi/openapi.yaml) has Project-scoped
  Execution reads but no Evidence route. [UsageRecord](../../../packages/contracts/schemas/UsageRecord.json)
  and [UsageReport](../../../packages/contracts/schemas/UsageReport.json) define
  public measurement presence/source semantics; they do not define a bundle.
- The separate #18 head `39386f7e9cd2782075d1cf865955a39ccef71a26`
  has `coordinator.ProjectAccess`, local-auth guards and HTTP composition.
  Its accepted ADR-0062 scopes public operations to the current authenticated
  compact Instance administrator. These files/decisions are absent on #19;
  their availability on an implementation base must be resolved, not assumed.

No real Provider, RustFS, cross-Organization deployment or public Evidence
access test establishes this proposed behavior. The review/queue evidence is in
the [continuation record](../../validation/m3/2026-11-14-linux-art-002-read-contract-proposal.md).
This supplements ADR-0069 and supersedes no accepted decision.

## Decision

**Proposed; owner approval required before implementing this public boundary.**

1. **Add one scoped read.** Use
   `GET /v1/projects/{projectId}/executions/{id}/evidence`, operation
   `getExecutionEvidence`, under the existing `/v1` major. It has no request
   body, caller-selected Organization, cursor, filter or partial-success mode.
   Authentication, Host/Origin checks and administrator authority reuse the
   compact API boundary. The route grants no execution or Approval authority.
   Organization and Project identity come from authenticated authoritative
   state, never from Worker claims or a content hash. Multi-user deployment
   must supply its actual scoped authority implementation before serving this
   route; the compact administrator rule is not a multi-tenant bypass.
2. **Authorize inside the read scope.** Resolve the attached Project and
   Execution ownership, every Step/context/Usage/Artifact/Approval reference,
   and the selected Approval audit entries in the same metadata transaction.
   A successful root lookup alone is insufficient. Check live compact session
   authority before beginning the read and again before returning. Membership
   and persisted scope are evaluated at the snapshot; later changes govern
   later reads/downloads, as ADR-0069 specifies. There is no claim of atomicity
   between an in-memory session recheck and response delivery.
3. **Use an explicit response projection.** The proposed envelope has required
   fields `metadata`, `metadataDigest`, and `availability`. `metadata` contains
   the Execution identity/scope and pinned definition, ordered Steps with their
   pinned Agent/Skill/context/input-hash/Artifact/Usage records, ordered
   Execution Events, Approvals and selected audit history. It separately names
   `terminal`, `consistency: "snapshot"`, and completeness entries per expected
   kind and applicable Step/attempt. New public record schemas are reviewed
   with fixtures before route registration; do not serialize domain structs.
   Reuse existing public shapes only where their meaning matches. In particular,
   map Usage to the existing string-valued source and decimal-cost semantics,
   retain attempt attribution and empty accepted receipts, and omit internal
   fencing/fingerprint fields and raw Step inputs.
4. **Keep absence explicit.** Completeness entries carry kind, applicable
   Step/attempt identity, `status` and a machine reason when status is not
   `captured`. Proposed statuses are `captured`, `pending`, `unavailable`,
   `expired`, `not_applicable`. Reasons distinguish `recorder_not_implemented`,
   `producer_failed` and `not_yet_produced`; an empty collection is not proof
   of non-applicability. Existing captures are retained by attempt, and a prior
   attempt cannot establish current-attempt coverage. Unknown Usage remains
   unknown. No synthetic checks, logs or success are inferred from Events.
   Unknown future status/reason values display as unknown to clients and never
   authorize a consequential action. A store/query error fails the whole query;
   it is not a completeness entry.
5. **Separate content, representation and availability.** Public metadata pins
   immutable content hashes and exact version/capture identities. Large content
   uses ArtifactStore references; metadata does not inline arbitrary output
   bytes. Existing context/definition representations and any inline text need
   explicit public field/redaction review; this proposal grants no secret
   disclosure. `metadataDigest` is a server-issued SHA-256 identity of its
   deterministic public metadata projection, not the internal bundle Digest or
   an Approval Evidence digest. Clients treat it as opaque; fixtures must prove
   deterministic server computation and changes when represented data changes.
   Existing decided Approval digests stay unchanged. `availability` reports each
   referenced blob's observed status (`available`, `missing`, `expired`,
   `not_checked`) and observation time when checked. A timeout/store failure
   is an unsuccessful query, not `missing`; `not_checked` asserts no successful
   check. Availability is outside `metadataDigest` and outside the PostgreSQL
   snapshot. A later authorized download can still fail; hashes are not tokens.
6. **Bound the single query.** Keep the existing maximum 30-second metadata
   transaction (or caller's earlier deadline). Proposed initial hard limits are
   10,000 returned metadata records and 16 MiB of encoded metadata. Count every
   nested Step, Event, audit entry, Approval, Artifact, context item, Skill and
   Usage receipt/report; apply read limits before fetching unbounded histories
   and an encoding limit before writing any HTTP success. Include availability
   checks in the caller deadline and bound their count by the returned references.
   Exceeding either limit returns HTTP 409 `conflict` with the generic machine
   detail `reason: evidence_query_limit`; no truncated bundle, raw count or
   leaked identifiers. These numerical limits and error mapping are proposed
   product choices, not existing service behavior. Larger exports require a
   separately reviewed snapshot/materialization contract, not ordinary cursors
   across changing metadata or a transaction held across requests.
7. **Preserve the existing error and cache boundary.** Existing authentication
   and Origin errors remain unchanged. A non-administrator receives 403
   `forbidden` before data access. After authority validation, nonexistent,
   malformed, foreign or inconsistent Project/Execution/child ownership returns
   the same generic 404 `not_found`, with no partial data. Storage/encoding/
   timeout failures return generic 500 `internal`; caller cancellation stops
   work without writing a partial success. Responses use `Cache-Control:
   no-store`, including errors. No server cache, presigned URL, new blob-download
   route, ETag, sealed export or late-enrichment policy is introduced here.

## Consequences

This makes the contract boundary reviewable without exposing internal structs
or equating terminal state with complete/retrievable Evidence. One bounded
response is simpler than snapshot tokens or persistent exports, but large runs
can fail the initial limit. The owner must approve that tradeoff and the exact
route/envelope/error mapping. The detailed schemas/fixtures and implementation
remain subsequent reviewed work; accepting this ADR alone creates no service.

Implementation spans existing contracts, fixture manifest and Go/TypeScript
parity, generated SDK types/tests, scoped application operations, transaction
adapters, HTTP registration/route guard and documentation. Resolve the #18/#19
composition base through the delivery workflow first. Domain code remains pure;
the Platform owns state and authorization, Workers only report facts, and
clients use the public API. No dependency, infrastructure, migration, Provider
change or prototype import is required by this proposal.

Git remains Project/source history; per [ADR-0017](0017-content-addressed-artifact-store-filesystem-or-s3.md),
ArtifactStore owns immutable output blobs and PostgreSQL their versions/history/
provenance. Filesystem is the compact adapter; RustFS is the reference
S3-compatible backend. The proposal does not create storage in a Project tree.

Required implementation evidence: two Projects with foreign child references;
authorization loss before read/return; transaction-bound authorization and
concurrent metadata writes; deterministic public digest independent of blob
availability; no raw inputs/secrets; missing/empty/historical Usage; every
completeness distinction; content removed after the read; exact limit and
limit-plus-one; timeout/rollback/no partial success; public fixtures/SDK and
route-drift checks on one integrated Linux base. Provider, native non-Linux and
M3 exit evidence stay separately owed. #124/#19/#20 remain open.

### Architecture review

Verdict: **needs human decision** on the route/projection, status vocabulary,
single-query limits and error mapping. The proposed boundaries follow accepted
ADRs 0003/0004/0017/0025/0069. No dependency or infrastructure is added. Important
implementation gates are scoped transaction composition (current internal
readers do not authorize), reviewed safe public text fields, and a shared
integration base (current #18 and #19 code is separate). Failure must return no
bundle rather than weaken authorization or substitute a gap. These are required
tests, not claims that the proposal already works.

## Alternatives considered

- **Serialize the internal bundle directly:** exposes internal names/measurement
  sources and lacks public scope, availability, completeness and size guarantees.
- **Expose a Usage-only endpoint as ART-002:** useful measurements alone cannot
  satisfy the accepted one-query reconstruction boundary or full audit chain.
- **Ordinary pagination with independent reads:** mixes revisions while claiming
  one bundle; materialized exports add persistence/lifecycle work not needed for
  the first bounded query.
- **Continue only internal hardening:** leaves the approved M3 public boundary
  undefined and downstream Approval/retention acceptance blocked.

## Approval

Proposed by: Codex on 2026-11-14 for #124/#19. Approved by:

Pending explicit product-owner approval. Neither merging this proposed document
nor approval of #400 accepts this contract or closes any implementation criterion.
