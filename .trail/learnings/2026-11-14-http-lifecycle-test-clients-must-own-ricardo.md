---
title: HTTP lifecycle test clients must own speculative connections
author: Ricardo
date: 2026-11-14
branch: test/170-http-lifecycle-capture
id: l-9dd96534
scope: repo
status: active
src: agent
touches:
  - docs/engineering/validation.md
  - services/platform/internal/platform/README.md
  - services/platform/internal/platform/http_linux_test.go
---

## Decided
In services/platform/internal/platform/http_linux_test.go, httpCall creates a request-owned transport with DisableKeepAlives. Each call is a standalone request; leaving reuse to the shared default transport can strand an unused connection when earlier responses close, making bounded lifecycle cleanup fail for client reasons.

## Tried and ruled out
Repeated race runs of TestHTTPDiscoveryAdmissionAndProductAuthentication failed during one-second shutdown despite zero admitted handlers. Temporary connection-state counts at shutdown entry showed one StateNew connection. Go 1.27 net/http closeIdleConns only treats StateNew as idle after more than five seconds. Increasing the cleanup budget would conceal the test client's ownership problem, so neither production HTTP behavior nor its timeout was changed. Temporary instrumentation was removed.

## Watch out
Check individual command logs: a shell invocation that runs a passing suite after a failing repeated test returns the last status. The test-owned diagnostic drain closes its queue only after Lifecycle.Run and HTTP producers have joined. This remains component integration, not production-entrypoint or full stdout/stderr capture evidence.
