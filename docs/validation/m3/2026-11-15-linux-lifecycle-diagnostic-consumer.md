# Linux lifecycle diagnostic consumer (#170)

Date: 2026-11-15. Runner: Codex, local Linux workspace. Base: #18 integration
`921f83a4ae9827fe4c874fff8077426a93ac3e24`, after merged PR #418.
Native #170 blockers #83/#84 and inherited #18 blocker #15 are closed.
PR base: `feat/18-wp-m3-06-public-api-events`. #170/#18/M3 remain open.

## Implementation and boundary review

`operational.Logger.ConsumeLifecycle` replaces manual test-owned drain loops
with a reusable consumer of the existing bounded lifecycle queue. It requires
trusted Instance/correlation context, uses the existing JSON/redaction writer,
drains until queue closure, and returns on cancellation or a safe write error.
The caller owns its goroutine, context, queue closure and final join; producers
must join before queue closure. Use a context that remains live through lifecycle
teardown to retain shutdown diagnostics. Cancellation abandons pending records;
an already started write can complete after cancellation.

Verdict: sound within accepted [ADR-0068](../../architecture/adr/0068-platform-process-readiness-and-bounded-shutdown.md).
No new service, supervisor, dependency, public contract, persistence change or
domain Event is introduced. IO stays outside Lane callbacks. The simpler direct
writer in the callback would permit sink latency to block lifecycle progress.
The existing handoff still drops on saturation. A stalled writer can block this
consumer, but cannot block producers through the handoff; composition must bound
the consumer join and exit on expiry. This helper cannot interrupt arbitrary IO.

## Evidence

- **Unit-tested:** ordered drain of a closed queue; trusted identity and known
  secret redaction; invalid composition rejected before consuming; cancellation
  with pending records and an idle queue; sink errors, short writes and panics
  return only the fixed safe writer error and stop consumption.
- **Integration-tested:** real Lane startup/panic/Stop failure diagnostics use a
  concurrent consumer, join producers, close the queue and join the consumer;
  JSON has trusted identity and omits the unregistered error/panic secret.
- **Integration-tested:** actual HTTP/Lifecycle adapter composition uses the
  reusable consumer after producer joins, retaining shared JSON, credential
  omission and request/process correlation checks with fake Project storage.
- **Unit-tested:** a blocked real consumer sink fills the bounded queue while
  concurrent slog producers finish; cancellation and sink release permit cleanup.

## Commands and results

Run from `services/platform` on Linux:

```sh
go test -race -count=20 -run 'TestLifecycleConsumer|TestLaneFailuresUseSafeLifecycleHandoff' ./internal/adapter/operational
go test -race -count=20 -run '^TestHTTPDiscoveryAdmissionAndProductAuthentication$' ./internal/platform
```

Both PASS, exit 0, no skips. Repository-root `make check`: PASS, exit 0,
including docs-check, Platform checks, 348 TypeScript contract tests,
346 matching Go/TypeScript fixture outcomes and 48 SDK tests.
Log: `/tmp/amaru-419-make-check.log`. Local evidence is not hosted CI.

`trail build`: PASS. `trail check --base 921f83a4ae9827fe4c874fff8077426a93ac3e24`
reports lexical conflicts in the validation guide and Platform README. The
change retains request-owned HTTP transports, SQL panic/process-failure limits,
receive-pump joining, readiness closure before cancellation and original-human
mutation checks. None of those implementations changes. Consumer ownership is
explicit: join producers before closing the queue and bound the final consumer
join; a stalled sink is not described as graceful shutdown. Advisory matches
introduce no storage filesystem imports, schema upgrades or scheduling changes.

## Limits

Production entrypoint wiring, process termination for a blocked diagnostic sink,
real Providers, PostgreSQL runtime and native macOS/Windows execution were not
validated by this adapter-only change. Cross-vet is a compatibility signal.
No automatic retry, guaranteed delivery or business outcome is inferred from
log severity. A failed/partial sink write can lose a diagnostic; consumption
stops without retrying. Existing process, Event, secret and #191 gates remain.
