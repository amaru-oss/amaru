package localauth

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Focused criterion tests for IAM-001 (#92). Fakes only: fixed clock, in-memory
// handler, no sockets, agents or external services.

func TestLaunchCodeCarries256BitsAndIsSingleUse(t *testing.T) {
	a, h, _ := setup(t)
	code := issue(t, a, h)
	raw, err := base64.RawURLEncoding.Strict().DecodeString(code)
	if err != nil || len(raw) != 32 {
		t.Fatalf("launch code must carry at least 256 bits, got %d bytes", len(raw))
	}
	if w := call(h, "GET", "/auth/launch?code="+code, nil); w.Code != 303 {
		t.Fatalf("first exchange status %d", w.Code)
	}
	if w := call(h, "GET", "/auth/launch?code="+code, nil); w.Code != 401 {
		t.Fatalf("sequential replay after successful exchange accepted: %d", w.Code)
	}
}

func TestLaunchCodeAcceptedJustBeforeSixtySecondExpiry(t *testing.T) {
	a, h, _ := setup(t)
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	code := issue(t, a, h)
	now = now.Add(CodeLifetime - time.Nanosecond)
	w := call(h, "GET", "/auth/launch?code="+code, nil)
	if w.Code != 303 || w.Header().Get("Location") != "/" {
		t.Fatalf("code rejected before 60s expiry: %d", w.Code)
	}
}

func TestBookmarkWithoutSessionExplainsAmaruOpen(t *testing.T) {
	a, h, count := setup(t)
	before := count.Load()
	w := call(h, "GET", "/", nil)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "Run `amaru open`") {
		t.Fatalf("bookmark without session did not explain amaru open: %d %q", w.Code, body)
	}
	if count.Load() != before {
		t.Fatal("bookmark without session dispatched downstream")
	}
	if strings.Contains(body, a.AdministratorSecret()) {
		t.Fatal("administrator secret in bookmark response")
	}
}

func TestWebSocketUpgradeRequiresExactAllowedOrigin(t *testing.T) {
	a, h, count := setup(t)
	c, _ := launch(t, a, h)
	for _, origin := range []string{"", "http://attacker.test", "null", "http://localhost:7778", "https://localhost:7777"} {
		headers := map[string]string{"Cookie": c.String(), "Upgrade": "websocket"}
		if origin != "" {
			headers["Origin"] = origin
		}
		before := count.Load()
		if w := call(h, "GET", "/projects/p/events", headers); w.Code != 403 || count.Load() != before {
			t.Fatalf("WebSocket upgrade with Origin %q not rejected without dispatch: %d", origin, w.Code)
		}
	}
}

func TestStateChangingMethodsRequireCSRFAndOrigin(t *testing.T) {
	a, h, count := setup(t)
	c, csrf := launch(t, a, h)
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			before := count.Load()
			base := map[string]string{"Cookie": c.String(), "Origin": "http://localhost:7777"}
			if w := call(h, method, "/command", base); w.Code != 403 || count.Load() != before {
				t.Fatalf("missing CSRF not rejected without dispatch: %d", w.Code)
			}
			if w := call(h, method, "/command", map[string]string{"Cookie": c.String(), "X-Amaru-CSRF": csrf}); w.Code != 403 || count.Load() != before {
				t.Fatalf("missing Origin not rejected without dispatch: %d", w.Code)
			}
			base["X-Amaru-CSRF"] = csrf
			if w := call(h, method, "/command", base); w.Code != 204 || count.Load() != before+1 {
				t.Fatalf("valid CSRF and Origin not dispatched once: %d", w.Code)
			}
		})
	}
}

func TestSecretsInQueryStringDoNotAuthenticate(t *testing.T) {
	a, h, count := setup(t)
	c, _ := launch(t, a, h)
	secret := a.AdministratorSecret()
	for _, path := range []string{
		"/projects/p/events?access_token=" + secret,
		"/projects/p/events?authorization=Bearer+" + secret,
		"/projects/p/events?amaru_session=" + c.Value,
	} {
		before := count.Load()
		w := call(h, "GET", path, map[string]string{"Upgrade": "websocket", "Origin": "http://localhost:7777"})
		if w.Code != 401 || count.Load() != before {
			t.Fatalf("query-string credential authenticated WebSocket at %s: %d", strings.SplitN(path, "?", 2)[0], w.Code)
		}
		if strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), c.Value) {
			t.Fatal("credential echoed in error")
		}
	}
	if w := call(h, "GET", "/projects/p/events?token="+secret, map[string]string{"Cookie": c.String(), "Origin": "http://localhost:7777", "Upgrade": "websocket"}); w.Code != 400 {
		t.Fatalf("token query accepted on cookie WebSocket: %d", w.Code)
	}
}

func TestIPv4HostileHostDoesNotConsumeLaunchCode(t *testing.T) {
	a, h, _ := setup(t)
	code := issue(t, a, h)
	for _, host := range []string{"attacker.test:7777", "127.0.0.1:7778", "localhost", "127.0.0.1.attacker.test:7777", "localhost:7777.attacker.test"} {
		r := httptest.NewRequest("GET", "http://localhost:7777/auth/launch?code="+code, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 || len(w.Result().Cookies()) != 0 {
			t.Fatalf("hostile Host %q accepted on launch: %d", host, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://localhost:7777/auth/launch?code="+code, nil)
	r.Host = "127.0.0.1:7777"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 303 {
		t.Fatalf("bound loopback Host rejected after hostile attempts: %d", w.Code)
	}
}

func TestLaunchCodeRequestsDoNotLeakIntoBodies(t *testing.T) {
	a, h, _ := setup(t)
	code := issue(t, a, h)
	w := call(h, "GET", "/auth/launch?code="+code, nil)
	if strings.Contains(w.Header().Get("Location"), code) || strings.Contains(w.Body.String(), code) {
		t.Fatal("launch code present in redirect target or body")
	}
}
