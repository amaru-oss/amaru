---
title: Mutation authorization must reject a changed human identity
author: Ricardo
date: 2026-11-14
branch: fix/191-mutation-session-io
id: l-ccc7a370
scope: repo
status: active
src: agent
touches:
  - docs/engineering/project-access.md
  - docs/engineering/validation.md
  - services/platform/internal/coordinator/project.go
---

## Decided
The owner required ProjectAccess Cancel, DecideApproval, ResumeStep and AbandonStep in services/platform/internal/coordinator/project.go to fail closed with ErrNotPermitted if the authenticated human ID differs between pre-resource-read and pre-coordinator authorization. Successful calls keep the original actor; resource storage errors and missing/foreign ownership errors retain precedence.

## Tried and ruled out
The initial services/platform/internal/coordinator/project_mutation_authority_test.go matrix allowed a replacement owner while keeping the first owner's attribution. The owner rejected that behavior: fresh authorization must not let another identity authorize the first identity's command. The final matrix requires an empty outcome and no coordinator replay/commit for an identity change.

## Watch out
Do not replace the original actor with the refreshed principal or move authorization ahead of resource-error handling. The check remains separate from the coordinator database commit; no atomic revocation guarantee is established. ADR-0070 Work scope remains outside this ADR-0062 fix.
