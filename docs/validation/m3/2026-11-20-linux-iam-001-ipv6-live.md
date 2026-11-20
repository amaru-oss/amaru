# #92 Linux live IPv6 discovery and native auth evidence

Bounded adapter evidence on 2026-11-20 at baseline `302103e79e438ba82ba8ffaeec14bc8396102a78`.
This adds a Linux integration test for existing discovery and local-auth
adapters. It does not change the production listener, startup wiring, contracts,
or security policy.

## Scope and evidence

`TestIPv6LiveDiscoveryAndNativeAuth` binds a real `tcp6` listener on `[::1]:0`,
publishes and reads private discovery for that bound endpoint, then probes its
health through `discovery.Live`. A test-only `http.Server` composes the existing
Auth handler with a minimal health response. The test exercises native bearer
launch-code issuance, launch-code exchange over IPv6, Secure/HttpOnly/Strict
cookie attributes, single-use replay rejection, and session read using the
issued cookie. The test uses generic failure messages and never prints
discovery contents, launch codes, cookies, bearer credentials, or response
bodies.

The test skips only when the TCP6 bind reports an unavailable IPv6 protocol or
loopback address. Other failures, including permission denial, remain failures.
Production bootstrap, dual-stack binding, browser behavior and the full product
API remain unvalidated by this test.

## Environment and command

- Linux `7.2.11-100.fc43.x86_64`, amd64; module-selected Go `1.27.0`.
- From `services/platform`:
  `GOCACHE=/tmp/amaru-92-ipv6-gocache go test -count=1 -run '^TestIPv6LiveDiscoveryAndNativeAuth$' ./internal/adapter/discovery`
  — PASS, exit 0; test ran without a skip.
- An initial sandboxed attempt failed at socket creation with `operation not
  permitted`; the same focused test passed when run outside the filesystem
  sandbox. This is local integration evidence, not CI evidence.

- Reviewed test, from `services/platform`:
  `GOCACHE=/tmp/amaru-after423-go-cache go test -race -v -count=10 -run '^TestIPv6LiveDiscoveryAndNativeAuth$' ./internal/adapter/discovery`
  passed all ten runs without a skip.
