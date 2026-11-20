//go:build linux

package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
)

// TestIPv6LiveDiscoveryAndNativeAuth uses a real TCP6 socket. Its handler is a
// test adapter, not production startup or a product server composition.
func TestIPv6LiveDiscoveryAndNativeAuth(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		if ipv6Unavailable(err) {
			t.Skip("IPv6 loopback is unavailable")
		}
		t.Fatalf("IPv6 listener setup failed (%T): %v", err, err)
	}
	defer listener.Close()

	auth, err := localauth.New(listener.Addr())
	if err != nil {
		t.Fatal("auth setup failed")
	}
	defer auth.Close()
	started := time.Now().UTC()
	const version = "ipv6-test"
	dir := filepath.Join(t.TempDir(), "instance")
	publication, err := Publish(dir, listener, auth.AdministratorSecret(), version, started)
	if err != nil {
		t.Fatal("discovery publication failed")
	}
	defer publication.Close()
	info, err := Read(dir)
	if err != nil || info.Endpoint != "http://"+listener.Addr().String() || info.Port != listener.Addr().(*net.TCPAddr).Port {
		t.Fatal("IPv6 discovery read failed")
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/health", auth.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "protocolVersion": 1, "platformVersion": version, "startedAt": started, "uptimeSec": 0})
	})))
	mux.Handle("/", auth.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })))
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	if !Live(context.Background(), info) {
		t.Fatal("published IPv6 endpoint health probe failed")
	}

	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	endpoint := info.Endpoint
	issue, err := http.NewRequest(http.MethodPost, endpoint+"/v1/auth/launch-codes", nil)
	if err != nil {
		t.Fatal("could not construct native request")
	}
	issue.Header.Set("Authorization", "Bearer "+info.AdministratorSecret)
	resp, err := client.Do(issue)
	if err != nil {
		t.Fatal("native launch-code issuance request failed")
	}
	var payload struct {
		Code string `json:"code"`
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || decodeErr != nil || payload.Code == "" {
		t.Fatal("native launch-code issuance failed")
	}
	launchURL := endpoint + "/auth/launch?code=" + url.QueryEscape(payload.Code)
	resp, err = client.Get(launchURL)
	if err != nil {
		t.Fatal("launch exchange request failed")
	}
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" || len(resp.Cookies()) != 1 {
		resp.Body.Close()
		t.Fatal("launch exchange failed")
	}
	cookie := resp.Cookies()[0]
	resp.Body.Close()
	if !cookie.Secure || !cookie.HttpOnly || cookie.Domain != "" || cookie.Path != "/" || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("session cookie attributes invalid")
	}
	resp, err = client.Get(launchURL)
	if err != nil {
		t.Fatal("launch replay request failed")
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("launch replay was accepted")
	}
	request, err := http.NewRequest(http.MethodGet, endpoint+"/v1/auth/session", nil)
	if err != nil {
		t.Fatal("could not construct session request")
	}
	request.AddCookie(cookie)
	resp, err = client.Do(request)
	if err != nil {
		t.Fatal("cookie session request failed")
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	decodeErr = json.NewDecoder(resp.Body).Decode(&session)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || decodeErr != nil || session.CSRFToken == "" {
		t.Fatal("cookie session exchange failed")
	}

	for _, tc := range []struct {
		method, csrf string
		want         int
	}{
		{http.MethodDelete, "", http.StatusForbidden},
		{http.MethodGet, "", http.StatusOK},
		{http.MethodDelete, session.CSRFToken, http.StatusNoContent},
		{http.MethodGet, "", http.StatusUnauthorized},
	} {
		req, err := http.NewRequest(tc.method, endpoint+"/v1/auth/session", nil)
		if err != nil {
			t.Fatal("could not construct session lifecycle request")
		}
		req.AddCookie(cookie)
		if tc.method == http.MethodDelete {
			req.Header.Set("Origin", endpoint)
		}
		if tc.csrf != "" {
			req.Header.Set("X-Amaru-CSRF", tc.csrf)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal("IPv6 session lifecycle request failed")
		}
		res.Body.Close()
		if res.StatusCode != tc.want {
			t.Fatalf("%s session returned %d, want %d", tc.method, res.StatusCode, tc.want)
		}
	}
}

func ipv6Unavailable(err error) bool {
	return errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EPROTONOSUPPORT) || errors.Is(err, syscall.EADDRNOTAVAIL)
}
