---
title: Keep validation temp directories outside the Git checkout
author: Ricardo
date: 2026-11-14
branch: feat/170-http-log-callback-panic
id: l-2eab50fa
scope: repo
status: active
src: agent
touches:
  - services/platform/README.md
  - services/platform/internal/adapter/operational/http.go
  - services/platform/internal/adapter/operational/http_test.go
---

## Decided
Use an external /home task directory for TMPDIR and check logs when /tmp is unavailable.

## Tried and ruled out
Repository-nested .task-tmp caused attach TestProjectApplicationWithGit, TestAttachRefusals and TestInspectInvalidRoots to fail: their plain t.TempDir directories inherited the checkout through git rev-parse --show-toplevel. All three passed with external TMPDIR; no attach or worker code was changed.

## Watch out
An earlier make check also timed out in embedded TestFailureAndPanicAreReported; its isolated rerun passed, but the cause was not established. Do not attribute that timeout to logging or claim a fix.
