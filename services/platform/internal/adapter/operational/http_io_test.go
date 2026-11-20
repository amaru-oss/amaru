package operational

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingResponse struct {
	*httptest.ResponseRecorder
	err   error
	short bool
}

func (w *failingResponse) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, w.err
}
func (w *failingResponse) FlushError() error                            { return w.err }
func (w *failingResponse) Hijack() (net.Conn, *bufio.ReadWriter, error) { return nil, nil, w.err }

func TestHTTPResponseIOFailureDiagnostics(t *testing.T) {
	for _, operation := range []string{"write", "short_write", "flush", "hijack"} {
		t.Run(operation, func(t *testing.T) {
			var output bytes.Buffer
			logger, _ := New(&output)
			raw := errors.New("unregistered-response-credential")
			writer := &failingResponse{ResponseRecorder: httptest.NewRecorder(), err: raw, short: operation == "short_write"}
			handler := logger.HTTP("instance", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				switch operation {
				case "write", "short_write":
					n, err := w.Write([]byte("private-response-body"))
					if writer.short {
						if n != len("private-response-body")-1 || err != nil {
							t.Fatal("short write changed")
						}
					} else if n != 0 || err != raw {
						t.Fatal("write error changed")
					}
				case "flush":
					if err := http.NewResponseController(w).Flush(); err != raw {
						t.Fatal("flush error changed")
					}
				case "hijack":
					if _, _, err := http.NewResponseController(w).Hijack(); err != raw {
						t.Fatal("hijack error changed")
					}
				}
			}), func(err error) { t.Error(err) })
			handler.ServeHTTP(writer, httptest.NewRequest("GET", "/?token=private-query", nil))
			recs := records(t, &output)
			if len(recs) != 1 {
				t.Fatalf("records: %d", len(recs))
			}
			rec := recs[0]
			if rec["outcome"] != "failed" || rec["cause"] != "response_io_failed" || rec["status"] != float64(http.StatusAccepted) || rec["correlationId"] == "" {
				t.Fatalf("incorrect diagnostic: %v", rec)
			}
			for _, secret := range []string{raw.Error(), "private-response-body", "private-query"} {
				if strings.Contains(output.String(), secret) {
					t.Fatal("response diagnostics leaked input")
				}
			}
		})
	}
}

func TestHTTPDeliberateAbortDiagnostic(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output)
	if err != nil {
		t.Fatal(err)
	}
	handler := logger.HTTP("instance", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}), func(err error) { t.Error(err) })
	func() {
		defer func() {
			if got := recover(); got != http.ErrAbortHandler {
				t.Errorf("abort was not forwarded as http.ErrAbortHandler")
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}()
	recs := records(t, &output)
	if len(recs) != 1 || recs[0]["outcome"] != "failed" || recs[0]["cause"] != "response_aborted" || recs[0]["status"] != nil {
		t.Fatalf("unexpected abort diagnostic: %v", recs)
	}
}
