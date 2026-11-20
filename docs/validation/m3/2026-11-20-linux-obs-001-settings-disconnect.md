# OBS-001 authenticated settings disconnect

Local Linux evidence for [#170](https://github.com/rluders/amaru/issues/170).

**Integration-tested:** `TestHTTPSettingsClientDisconnectDiagnostic` sends an authenticated settings GET through Platform HTTP on loopback and cancels it while the fake settings store is active. The test waits for the store to see cancellation before starting Platform shutdown. The JSON diagnostic retains the rejection outcome and includes the static `request_canceled` cause with the trusted request correlation. The administrator credential, query fixture and client-supplied correlation are absent.

**Unit-tested:** `TestHTTPCancellationDiagnostics` covers cancellation and deadlines on both successful and rejected responses. Response I/O failure retains precedence when one occurs.

- From `services/platform`, `GOCACHE=/tmp/amaru-after423-go-cache go test -race -count=20 ./internal/platform ./internal/adapter/operational -run '^TestHTTP(SettingsClientDisconnectDiagnostic|CancellationDiagnostics)$'` passed (exit 0).
- `GOCACHE=/tmp/amaru-after423-go-cache make check` passed (exit 0).

Production server wiring and real Provider output capture remain open.
