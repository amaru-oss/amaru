# OBS-001 execution command diagnostics

Date: 2026-11-16. Local Linux sandbox; fake-backed evidence, not hosted CI or real Provider validation.

## Change and evidence

`operational.Commands` decorates coordinator Submit/Cancel with automatic JSON completion records. Successful Submit obtains Project/Execution identity from its result; other records retain trusted command context. Idempotency keys, cancellation reasons, raw errors and URLs are omitted. Known error categories supply fixed rejection causes; unknown failures report `command dependency failed`. Logging errors/panics do not replace command results or roll back commits. The existing Runner observer now supplies a static failure cause without inspecting Provider output.

**Unit-tested:** `TestCommandsProviderFailureAndCancellation` invokes decorated commands and an observed fake Runner, parses successful command, Provider failure, cancellation and rejection records, checks correlation and secret absence, and compares domain/audit history across rejection. `TestCommandFailureOmitsCredentialBearingError` injects an unregistered credential-bearing URL through a fake store, verifies safe diagnostics and unchanged returned error identity. `TestCommandSinkFailurePreservesCommitAndReplay` covers committed command/replay behavior despite sink panic and unchanged Event history.

## Integration branch validation

- `GOCACHE=/tmp/amaru-after423-go-cache make check` passed (exit 0) on Linux with loopback available. Platform tests and host, Windows, macOS and PostgreSQL-tagged vet ran.
- **Integration-tested:** `TestExecutionDiagnosticChannels` captures child-process stdout and stderr from the Coordinator and Worker composed with a fake Runner. It covers success, failure, panic and cancellation and requires a static cause in the failure record.

## Limits

This is adapter composition with fakes. Production command/Worker/server wiring remains the existing #18 gate; this record does not establish full #170 acceptance. Cancellation/rejection identity must already be supplied by trusted composition; request fields are not a source of trusted identity. Command return values (including errors) are preserved for existing callers; they are not safe log inputs. Sink failures may lose diagnostics and must not trigger command retries. Real Provider output channels, PostgreSQL runtime and native macOS/Windows are not validated here. No dependency, domain Event schema or audit writer changed.
