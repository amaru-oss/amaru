package operational

import (
	"bufio"
	"context"
	"crypto/rand"
	"net"
	"net/http"
)

// HTTP must wrap auth, so rejected requests and launch exchanges are captured.
// onError is required and receives only safe logger errors; it must not log raw
// requests or retry commands. Neither request nor response credentials are read.
func (l *Logger) HTTP(instanceID string, next http.Handler, onError func(error)) http.Handler {
	if instanceID == "" || next == nil || onError == nil {
		panic("operational HTTP requires instance, handler and error callback")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithIdentity(r.Context(), Identity{InstanceID: instanceID, CorrelationID: rand.Text()})
		rw := &response{ResponseWriter: w, status: http.StatusOK}
		outcome := "panicked"
		cause := ""
		defer func() {
			// The error callback can panic too; keep its values out of net/http logs.
			defer func() {
				if recover() != nil {
					panic(http.ErrAbortHandler)
				}
			}()
			recovered := recover()
			if recovered == http.ErrAbortHandler {
				outcome, cause = "failed", "response_aborted"
			}
			if recovered != nil && !rw.wrote {
				rw.status = 0
			}
			if err := l.Write(ctx, Record{Operation: "http.request", Outcome: outcome, Cause: cause, Status: rw.status}); err != nil {
				onError(err)
			}
			// Prevent net/http from printing a raw panic value or stack to ErrorLog.
			if recovered != nil {
				panic(http.ErrAbortHandler)
			}
		}()
		next.ServeHTTP(rw, r.WithContext(ctx))
		outcome = "succeeded"
		if ctx.Err() != nil {
			outcome, cause = "canceled", "request_canceled"
			if ctx.Err() == context.DeadlineExceeded {
				cause = "request_deadline_exceeded"
			}
		}
		if rw.status >= 400 {
			outcome = "rejected"
		}
		if rw.ioFailed {
			outcome = "failed"
			cause = "response_io_failed"
		}
	})
}

type response struct {
	http.ResponseWriter
	status   int
	wrote    bool
	ioFailed bool
}

func (w *response) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *response) WriteHeader(status int) {
	if w.wrote {
		return
	}
	// Like net/http, treat 101 as final: no later header replaces the upgrade.
	if status >= 100 && status < 200 && status != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	// A rejected header can panic; leave status open for handler recovery.
	w.ResponseWriter.WriteHeader(status)
	w.status = status
	w.wrote = true
}
func (w *response) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	// Record only failure presence, never the error or response body. Preserve
	// the writer's result for the handler; diagnostics must not retry delivery.
	if err != nil || n != len(p) {
		w.ioFailed = true
	}
	return n, err
}

// Preserve WebSocket hijacking and report the upgrade separately from HTTP 200.
func (w *response) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		if !w.wrote {
			w.status = http.StatusSwitchingProtocols
			w.wrote = true
		}
	} else {
		w.ioFailed = true
	}
	return conn, rw, err
}
func (w *response) FlushError() error {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if err != nil {
		w.ioFailed = true
	}
	return err
}
