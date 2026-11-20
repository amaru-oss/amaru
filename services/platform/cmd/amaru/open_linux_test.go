//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/discovery"
	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
)

func TestOpenExchangesOneTimeCode(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	server := httptest.NewUnstartedServer(nil)
	auth, err := localauth.New(server.Listener.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Close()
	started := time.Now().UTC()
	server.Config.Handler = auth.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("health probe sent credentials")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "protocolVersion": 1, "platformVersion": "test",
			"startedAt": started, "uptimeSec": 0,
		})
	}))
	server.Start()
	defer server.Close()
	dir, err := discovery.PrepareStateDirectory()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := discovery.Publish(dir, server.Listener, auth.AdministratorSecret(), "test", started)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	var stderr bytes.Buffer
	handoffs := 0
	open := func(ctx context.Context, raw string) error {
		handoffs++
		launch, err := url.Parse(raw)
		if err != nil || launch.Scheme+"://"+launch.Host != server.URL || launch.Path != "/auth/launch" ||
			len(launch.Query()) != 1 || launch.Query().Get("code") == "" || strings.Contains(raw, auth.AdministratorSecret()) {
			return errors.New("unexpected browser destination")
		}
		client := server.Client()
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return errors.New("invalid browser request")
		}
		for _, want := range []int{http.StatusSeeOther, http.StatusUnauthorized} {
			resp, err := client.Do(req)
			if err != nil {
				return errors.New("browser request failed")
			}
			resp.Body.Close()
			if resp.StatusCode != want {
				return errors.New("unexpected launch response")
			}
		}
		return nil
	}
	if code := run(t.Context(), []string{"open"}, &stderr, open); code != 0 || handoffs != 1 || stderr.Len() != 0 {
		t.Fatalf("open exit=%d handoffs=%d stderr bytes=%d", code, handoffs, stderr.Len())
	}
	if err := pub.Close(); err != nil {
		t.Fatal(err)
	}
	if code := run(t.Context(), []string{"open"}, &stderr, open); code != 1 || handoffs != 1 {
		t.Fatalf("missing Instance exit=%d handoffs=%d", code, handoffs)
	}
}

func TestLaunchResponseRejected(t *testing.T) {
	code := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	future := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"short code", `{"code":"a","expiresAt":"` + future + `"}`, 201},
		{"expired", `{"code":"` + code + `","expiresAt":"2000-01-01T00:00:00Z"}`, 201},
		{"trailing JSON", `{"code":"` + code + `","expiresAt":"` + future + `"} {}`, 201},
		{"oversized", strings.Repeat(" ", 4097), 201},
		{"redirect", "", 307},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/v1/auth/launch-codes" || r.Method != http.MethodPost ||
					r.Header.Get("Authorization") != "Bearer test-secret" || r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" {
					t.Error("unexpected native launch request")
				}
				w.Header().Set("Location", "/unexpected-redirect")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			launch, err := requestLaunchURL(t.Context(), discovery.Info{Endpoint: server.URL, AdministratorSecret: "test-secret"})
			if launch != "" || err == nil || requests.Load() != 1 {
				t.Errorf("unsafe response accepted or request repeated")
			}
		})
	}
}

func TestOpenBrowserPassesURLAsSingleArgument(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	opener := filepath.Join(bin, "xdg-open")
	if err := os.WriteFile(opener, []byte("#!/bin/sh\n[ \"$#\" = 1 ] && [ \"$1\" = http://127.0.0.1/test ]\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := openBrowser(t.Context(), "http://127.0.0.1/test"); err != nil {
		t.Fatal("browser opener did not receive the expected URL")
	}
}
