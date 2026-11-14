# OBS-001 HTTP log-error callback panic containment

Date: 2026-11-14. Host: Linux amd64, Go 1.26.0. Base: #18 integration `05991a2`.

## Scope and evidence

Implemented-and-verified as a partial #170 slice: the operational HTTP middleware converts a panic from its log-error callback to `http.ErrAbortHandler`. Raw callback values cannot escape this boundary into the HTTP server error log. A committed command is not retried or rolled back; the connection aborts, so the client may not receive its response.

The loopback regression uses a failing log sink and a callback that panics with an unregistered credential fixture. It exercises both a successful handler and a handler panic, checks the safe sink error, verifies connection abortion, and requires an empty server error log. Both cases failed before the fix with `raw callback panic diagnostic escaped` and pass after it. This is **integration-tested**, not a running-product acceptance run.

## Commands and exact outcomes

- Before the fix: `TMPDIR="$PWD/.task-tmp" go test -C services/platform ./internal/adapter/operational -run TestHTTPLogErrorCallbackPanicOmitsRawDiagnostic -count=1` — exit 1, both regression cases failed as expected.
- After the fix: `TMPDIR="$PWD/.task-tmp" go test -C services/platform -race ./internal/adapter/operational ./internal/platform -count=1` — exit 0, both packages passed.
- First `TMPDIR="$PWD/.task-tmp" make check` — exit 2: embedded Worker `TestFailureAndPanicAreReported` timed out waiting for both Steps to fail. Its isolated `go test -C services/platform ./internal/worker/embedded -run TestFailureAndPanicAreReported -count=1` with the same TMPDIR passed (exit 0); cause remains unresolved. No Worker code changed.
- Second `TMPDIR="$PWD/.task-tmp" make check` — exit 2: attach tests `TestProjectApplicationWithGit`, `TestAttachRefusals` and `TestInspectInvalidRoots` accepted nominal non-repository temp roots because Git discovered the parent checkout.
- `TMPDIR=/home/rluders/amaru-m3-170-tmp go test -C services/platform ./internal/attach -run 'TestProjectApplicationWithGit|TestAttachRefusals|TestInspectInvalidRoots' -count=1` — exit 0. All three passed with temp files and log outside the checkout.
- `TMPDIR=/home/rluders/amaru-m3-170-tmp make check` — exit 0, including 346 Go/TypeScript fixture outcomes with identical results. Full log: external task directory `make-check.log`. These are local results, not hosted CI.

## Limits

#170 and #18 remain open for composed product acceptance. Native macOS/Windows runtime, PostgreSQL-backed tests and real Providers were not run for this HTTP-only slice. Cross-compilation vet checks in `make check` are not native OS validation. The previous Worker timeout is recorded rather than represented as fixed. No new dependency, contract, architecture decision or prototype migration.
