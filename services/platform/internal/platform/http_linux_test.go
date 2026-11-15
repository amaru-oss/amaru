//go:build linux

package platform

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/discovery"
	"github.com/rluders/amaru/services/platform/internal/adapter/httpapi"
	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/adapter/operational"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
	"github.com/rluders/lane/v2"
)

func httpOwner(t *testing.T, dir string, l *Lifecycle, factory func(*localauth.Auth) (http.Handler, error)) (*HTTP, *bytes.Buffer) {
	t.Helper()
	logs := &bytes.Buffer{}
	logger, err := operational.New(logs)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewHTTP(HTTPConfig{StateDir: dir, Version: "test-build", InstanceID: "test-instance", Lifecycle: l, Logger: logger, OnLogError: func(error) {}, Handler: factory})
	if err != nil {
		t.Fatal(err)
	}
	return h, logs
}

func startHTTP(t *testing.T, h *HTTP, extra ...lane.Runner) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.config.Lifecycle.Run(ctx, append([]lane.Runner{h}, extra...)) }()
	t.Cleanup(cancel)
	return cancel, done
}

func readyHTTP(t *testing.T, h *HTTP) discovery.Info {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := h.WaitReady(ctx); err != nil {
		t.Fatal("HTTP not ready")
	}
	for !h.config.Lifecycle.Ready() {
		if ctx.Err() != nil {
			t.Fatal("aggregate readiness did not complete")
		}
		time.Sleep(time.Millisecond)
	}
	info, err := discovery.Read(h.config.StateDir)
	if err != nil {
		t.Fatal("private discovery unavailable")
	}
	return info
}

func httpCall(t *testing.T, info discovery.Info, method, path, body string, configure func(*http.Request)) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, info.Endpoint+path, strings.NewReader(body))
	if err != nil {
		t.Fatal("request creation failed")
	}
	if configure != nil {
		configure(req)
	}
	// Each call owns one request; shared reuse can leave speculative StateNew dials.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("loopback request failed")
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestHTTPDiscoveryAdmissionAndProductAuthentication(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	queue := make(chan operational.Record, 128)
	l, _ := NewLifecycle(time.Now().Add(3*time.Second), time.Second, queue)
	productIdentity := make(chan operational.Identity, 16)
	projects := fakes.NewProjectStore()
	key := domain.ProjectKey("id:01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if err := projects.Create(context.Background(), domain.Project{Key: key, Name: "one", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	h, logs := httpOwner(t, dir, l, func(auth *localauth.Auth) (http.Handler, error) {
		access, err := coordinator.NewProjectAccess(coordinator.New(fakes.NewExecutionStore(), time.Now), projects, auth.Principal)
		if err != nil {
			return nil, err
		}
		handler, err := httpapi.SettingsHandler(auth, access, projects)
		if err != nil {
			return nil, err
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			productIdentity <- operational.IdentityFrom(r.Context())
			handler.ServeHTTP(w, r)
		}), nil
	})
	h.config.OnLogError = func(error) { t.Error("safe diagnostic write failed") }
	gate := make(chan struct{})
	cancel, done := startHTTP(t, h, dependent{ready: func(ctx context.Context) error {
		select {
		case <-gate:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}})
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := h.WaitReady(ctx); err != nil {
		t.Fatal("HTTP startup failed")
	}
	info, err := discovery.Read(dir)
	if err != nil {
		t.Fatal("discovery missing")
	}
	resp := httpCall(t, info, "GET", "/v1/health", "", nil)
	if resp.StatusCode != 503 {
		t.Fatal("admission opened before aggregate readiness")
	}
	var unavailable struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.NewDecoder(resp.Body).Decode(&unavailable) != nil || unavailable.Error.Code != "internal" || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Amaru-Protocol") != "1" {
		t.Fatal("readiness rejection does not use the existing error contract")
	}
	resp.Body.Close()
	close(gate)
	info = readyHTTP(t, h)
	if !discovery.Live(context.Background(), info) {
		t.Fatal("health/discovery metadata disagree")
	}
	native := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+info.AdministratorSecret) }
	path := "/v1/projects/" + url.PathEscape(string(key)) + "/settings"
	resp = httpCall(t, info, "GET", path, "", nil)
	if resp.StatusCode != 401 {
		t.Fatal("missing credentials admitted")
	}
	resp.Body.Close()
	resp = httpCall(t, info, "GET", path+"?credential=opaque-query-fixture", "opaque-body-fixture", func(r *http.Request) {
		r.Header.Set("X-Correlation-ID", "untrusted-correlation-fixture")
		r.Header.Set("Authorization", "Bearer opaque-bearer-fixture")
		r.Header.Set("Cookie", "amaru_session=opaque-cookie-fixture")
	})
	if resp.StatusCode != 401 {
		t.Fatal("fixture credentials admitted")
	}
	resp.Body.Close()
	resp = httpCall(t, info, "GET", path, "", func(r *http.Request) { native(r); r.Host = "hostile.invalid" })
	if resp.StatusCode != 403 {
		t.Fatal("hostile Host admitted")
	}
	resp.Body.Close()
	resp = httpCall(t, info, "POST", "/v1/auth/launch-codes", "", native)
	var launch struct {
		Code string `json:"code"`
	}
	if resp.StatusCode != 201 || json.NewDecoder(resp.Body).Decode(&launch) != nil || launch.Code == "" {
		t.Fatal("native issuance failed")
	}
	resp.Body.Close()
	resp = httpCall(t, info, "GET", "/auth/launch?code="+url.QueryEscape(launch.Code), "", nil)
	cookies := resp.Cookies()
	if resp.StatusCode != 303 || resp.Header.Get("Location") != "/" || len(cookies) != 1 {
		t.Fatal("launch exchange failed")
	}
	resp.Body.Close()
	browser := func(r *http.Request) { r.AddCookie(cookies[0]) }
	resp = httpCall(t, info, "GET", "/v1/auth/session", "", browser)
	var session struct {
		CSRF string `json:"csrfToken"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&session) != nil {
		t.Fatal("session read failed")
	}
	resp.Body.Close()
	resp = httpCall(t, info, "PATCH", path, `{"importClaudeSkills":true}`, func(r *http.Request) {
		browser(r)
		r.Header.Set("Origin", info.Endpoint)
		r.Header.Set("X-Amaru-CSRF", session.CSRF)
		r.Header.Set("Content-Type", "application/json")
	})
	if resp.StatusCode != 200 {
		t.Fatal("authorized product mutation failed")
	}
	resp.Body.Close()
	settings, err := projects.Settings(context.Background(), key)
	if err != nil || !settings.ImportClaudeSkills {
		t.Fatal("product mutation did not persist")
	}
	resp = httpCall(t, info, "POST", "/v1/auth/launch-codes", "", func(r *http.Request) { native(r); r.Header.Set("Origin", info.Endpoint) })
	if resp.StatusCode != 401 {
		t.Fatal("mixed native/browser request admitted")
	}
	resp.Body.Close()
	cancel()
	if err := result(t, done); err != nil {
		t.Fatal("shutdown failed")
	}
	if _, err := os.Stat(filepath.Join(dir, discovery.FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("discovery retained after serving stopped")
	}
	if h.auth.AdministratorSecret() != "" {
		t.Fatal("administrator credential retained")
	}
	// HTTP and Lane producers have joined; only this composition caller closes/drains.
	close(queue)
	trusted := operational.WithIdentity(context.Background(), operational.Identity{InstanceID: "test-instance", CorrelationID: "trusted-lifecycle"})
	lifecycleCount := len(queue)
	if lifecycleCount == 0 || lifecycleCount == cap(queue) {
		t.Fatal("missing lifecycle diagnostics or insufficient queue capacity")
	}
	if err := h.config.Logger.ConsumeLifecycle(trusted, queue); err != nil {
		t.Fatal("lifecycle diagnostic consumer failed")
	}
	for _, secret := range []string{info.AdministratorSecret, launch.Code, cookies[0].Value, session.CSRF,
		"opaque-query-fixture", "opaque-body-fixture", "opaque-bearer-fixture", "opaque-cookie-fixture", "untrusted-correlation-fixture"} {
		if secret == "" || strings.Contains(logs.String(), secret) {
			t.Fatal("empty fixture or credential/untrusted request data leaked to diagnostics")
		}
	}
	var identity operational.Identity
	select {
	case identity = <-productIdentity:
	default:
		t.Fatal("successful mutation did not reach the product handler")
	}
	if len(productIdentity) != 0 || identity.InstanceID != "test-instance" || identity.CorrelationID == "" {
		t.Fatal("rejected requests reached product or product identity missing")
	}
	statuses := map[int]int{503: 1, 401: 3, 403: 1, 201: 1, 303: 1, 200: 3}
	seen := map[string]bool{}
	matchedProduct := false
	for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		var record map[string]any
		if json.Unmarshal(line, &record) != nil || record["instanceId"] != "test-instance" {
			t.Fatal("invalid JSON or missing trusted Instance identity")
		}
		timestamp, ok := record["time"].(string)
		if !ok {
			t.Fatal("missing diagnostic timestamp")
		}
		if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
			t.Fatal("invalid diagnostic timestamp")
		}
		if record["operation"] == "process.lifecycle" {
			if len(record) != 5 || record["correlationId"] != "trusted-lifecycle" {
				t.Fatal("lifecycle record contains unexpected fields or identity")
			}
			switch record["outcome"] {
			case "debug", "info", "warning":
			default:
				t.Fatal("unexpected failure on successful lifecycle cleanup")
			}
			lifecycleCount--
			continue
		}
		correlation, ok := record["correlationId"].(string)
		if len(record) != 6 || record["operation"] != "http.request" || !ok || correlation == "" || seen[correlation] || correlation == "trusted-lifecycle" {
			t.Fatal("HTTP record has unexpected fields or missing/duplicate correlation")
		}
		seen[correlation] = true
		status, ok := record["status"].(float64)
		if !ok || statuses[int(status)] == 0 {
			t.Fatal("unexpected HTTP diagnostic status")
		}
		statuses[int(status)]--
		outcome := "succeeded"
		if status >= 400 {
			outcome = "rejected"
		}
		if record["outcome"] != outcome {
			t.Fatal("HTTP diagnostic outcome disagrees with response")
		}
		if correlation == identity.CorrelationID {
			matchedProduct = status == 200
		}
	}
	for _, remaining := range statuses {
		if remaining != 0 {
			t.Fatal("missing HTTP diagnostic")
		}
	}
	if !matchedProduct || lifecycleCount != 0 {
		t.Fatal("product correlation or lifecycle records lost in shared sink")
	}
}

func TestHTTPPartialStartupRollsBack(t *testing.T) {
	for _, failure := range []string{"handler", "panic", "nil", "permissions", "locked"} {
		t.Run(failure, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "state")
			if failure == "permissions" {
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			l, _ := NewLifecycle(time.Now().Add(time.Second), time.Second, nil)
			factory := func(*localauth.Auth) (http.Handler, error) {
				switch failure {
				case "handler":
					return nil, errors.New("private handler failure")
				case "panic":
					panic("private handler panic")
				case "nil":
					return nil, nil
				}
				return http.NotFoundHandler(), nil
			}
			if failure == "locked" {
				listener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				auth, err := localauth.New(listener.Addr())
				if err != nil {
					t.Fatal(err)
				}
				defer auth.Close()
				previous, err := discovery.Publish(dir, listener, auth.AdministratorSecret(), "previous", time.Now())
				if err != nil {
					t.Fatal("previous publication failed")
				}
				defer previous.Close()
			}
			h, _ := httpOwner(t, dir, l, factory)
			_, done := startHTTP(t, h)
			if err := result(t, done); err == nil {
				t.Fatal("failed startup succeeded")
			}
			if l.Ready() || h.WaitReady(context.Background()) == nil {
				t.Fatal("failed initialization became ready")
			}
			if h.auth != nil && h.auth.AdministratorSecret() != "" {
				t.Fatal("partial Auth not closed")
			}
			if h.listener != nil {
				c, err := net.DialTimeout("tcp", h.listener.Addr().String(), 100*time.Millisecond)
				if err == nil {
					c.Close()
					t.Fatal("partial listener not closed")
				}
			}
			if failure == "locked" {
				info, err := discovery.Read(dir)
				if err != nil || info.PlatformVersion != "previous" {
					t.Fatal("failed startup replaced previous owner")
				}
			} else if _, err := os.Stat(filepath.Join(dir, discovery.FileName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed startup published discovery")
			}
		})
	}
}

func TestHTTPShutdownDrainsOrdinaryRequestsAndClosesStreams(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	l, _ := NewLifecycle(time.Now().Add(time.Second), 2*time.Second, nil)
	ordinary, release, stream, cancelled := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	h, _ := httpOwner(t, dir, l, func(*localauth.Auth) (http.Handler, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ordinary" {
				close(ordinary)
				select {
				case <-release:
				case <-r.Context().Done():
					t.Error("ordinary request cancelled before drain")
				}
				w.WriteHeader(204)
				return
			}
			conn, rw, err := http.NewResponseController(w).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
			_ = rw.Flush()
			close(stream)
			<-r.Context().Done()
			close(cancelled)
		}), nil
	})
	cancel, done := startHTTP(t, h)
	info := readyHTTP(t, h)
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		req, _ := http.NewRequest("GET", info.Endpoint+"/ordinary", nil)
		req.Header.Set("Authorization", "Bearer "+info.AdministratorSecret)
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Error("ordinary request failed")
			return
		}
		resp.Body.Close()
		if resp.StatusCode != 204 {
			t.Error("ordinary request did not drain")
		}
	}()
	wait(t, ordinary)
	conn, err := net.Dial("tcp", h.listener.Addr().String())
	if err != nil {
		t.Fatal("stream connection failed")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, err = fmt.Fprintf(conn, "GET /stream HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n", h.listener.Addr().String(), info.AdministratorSecret)
	if err != nil {
		t.Fatal("stream request failed")
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil || resp.StatusCode != 101 {
		t.Fatal("stream upgrade failed")
	}
	wait(t, stream)
	cancel()
	wait(t, cancelled)
	if _, err := discovery.Read(dir); err != nil {
		t.Fatal("publication removed while request still draining")
	}
	w := httptest.NewRecorder()
	h.server.Handler.ServeHTTP(w, httptest.NewRequest("GET", info.Endpoint+"/ordinary", nil))
	if w.Code != 503 {
		t.Fatal("shutdown admitted a request")
	}
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("hijacked stream remained open")
	}
	close(release)
	wait(t, requestDone)
	if err := result(t, done); err != nil {
		t.Fatal("draining shutdown failed")
	}
	h.mu.Lock()
	remaining := len(h.hijacked)
	h.mu.Unlock()
	if remaining != 0 {
		t.Fatal("closed hijacked connection retained")
	}
	if _, err := discovery.Read(dir); err == nil {
		t.Fatal("publication retained after drain")
	}
}

func TestHTTPRestartRotatesCredentials(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	var old discovery.Info
	for range 2 {
		l, _ := NewLifecycle(time.Now().Add(time.Second), time.Second, nil)
		h, _ := httpOwner(t, dir, l, func(*localauth.Auth) (http.Handler, error) {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), nil
		})
		cancel, done := startHTTP(t, h)
		info := readyHTTP(t, h)
		if old.AdministratorSecret != "" {
			if info.AdministratorSecret == old.AdministratorSecret {
				t.Fatal("restart reused credentials")
			}
			resp := httpCall(t, info, "GET", "/product", "", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+old.AdministratorSecret) })
			if resp.StatusCode != 401 {
				t.Fatal("old administrator credential survived restart")
			}
			resp.Body.Close()
		}
		cancel()
		if err := result(t, done); err != nil {
			t.Fatal("shutdown failed")
		}
		old = info
	}
}

func TestHTTPPrerequisiteFailureAcquiresNothing(t *testing.T) {
	l, _ := NewLifecycle(time.Now().Add(time.Second), time.Second, nil)
	dir := filepath.Join(t.TempDir(), "state")
	h, _ := httpOwner(t, dir, l, func(*localauth.Auth) (http.Handler, error) {
		t.Error("HTTP constructed before prerequisite readiness")
		return http.NotFoundHandler(), nil
	})
	err := l.Run(context.Background(), []lane.Runner{dependent{ready: func(context.Context) error { return errors.New("prerequisite failed") }}}, []lane.Runner{h})
	if err == nil || h.listener != nil || h.auth != nil {
		t.Fatal("failed prerequisite acquired HTTP resources")
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed prerequisite created state directory")
	}
}

func TestHTTPBoundedShutdownCancelsAndPreservesPublicationUntilJoin(t *testing.T) {
	l, _ := NewLifecycle(time.Now().Add(time.Second), 75*time.Millisecond, nil)
	dir := filepath.Join(t.TempDir(), "state")
	entered, cancelled, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	h, _ := httpOwner(t, dir, l, func(*localauth.Auth) (http.Handler, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer close(exited)
			close(entered)
			<-r.Context().Done()
			close(cancelled)
			<-release
		}), nil
	})
	cancel, done := startHTTP(t, h)
	info := readyHTTP(t, h)
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		req, _ := http.NewRequest("GET", info.Endpoint+"/blocked", nil)
		req.Header.Set("Authorization", "Bearer "+info.AdministratorSecret)
		client := &http.Client{Timeout: time.Second}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
	wait(t, entered)
	cancel()
	if err := result(t, done); err == nil {
		t.Fatal("cleanup deadline was reported as success")
	}
	wait(t, cancelled)
	wait(t, clientDone)
	if _, err := discovery.Read(dir); err != nil {
		t.Fatal("publication removed before handler joined")
	}
	close(release)
	wait(t, exited)
	wait(t, h.stopped)
	// Production must exit after timeout. The test releases abandoned resources
	// only after the uncooperative handler has ended, without restarting an owner.
	h.cancelRequests()
	h.auth.Close()
	if err := h.publication.Close(); err != nil {
		t.Fatal("test cleanup failed")
	}
}

func TestHTTPUnexpectedServeExitFailsLifecycle(t *testing.T) {
	l, _ := NewLifecycle(time.Now().Add(time.Second), time.Second, nil)
	h, _ := httpOwner(t, filepath.Join(t.TempDir(), "state"), l, func(*localauth.Auth) (http.Handler, error) { return http.NotFoundHandler(), nil })
	_, done := startHTTP(t, h)
	readyHTTP(t, h)
	_ = h.listener.Close()
	if err := result(t, done); err == nil || l.Ready() {
		t.Fatal("essential serving failure did not fail lifecycle")
	}
	if _, err := discovery.Read(h.config.StateDir); err == nil {
		t.Fatal("failed server retained publication after join")
	}
}
