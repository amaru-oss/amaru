package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
)

type settingsReadHook struct {
	ports.SettingsStore
	after func()
}

type settingsUpdateHook struct {
	ports.SettingsStore
	after func()
}

func (s settingsUpdateHook) UpdateSettings(ctx context.Context, key domain.ProjectKey, patch domain.UpdateSettings) (domain.ProjectSettings, error) {
	value, err := s.SettingsStore.UpdateSettings(ctx, key, patch)
	s.after()
	return value, err
}

func TestSettingsSessionLogoutDuringWrite(t *testing.T) {
	testSettingsSessionLogoutDuringWrite(t, fakes.NewProjectStore())
}

func testSettingsSessionLogoutDuringWrite(t *testing.T, store interface {
	ports.ProjectStore
	ports.SettingsStore
}) {
	t.Helper()
	for _, key := range []domain.ProjectKey{projectA, projectB} {
		if err := store.Create(context.Background(), domain.Project{Key: key, Name: "same", CreatedAt: time.Now()}); err != nil {
			t.Fatal("cannot create settings test Project")
		}
	}
	auth, err := localauth.New(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7777})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auth.Close)
	access, err := coordinator.NewProjectAccess(coordinator.New(fakes.NewExecutionStore(), time.Now), store, auth.Principal)
	if err != nil {
		t.Fatal(err)
	}
	after := func() {}
	h, err := SettingsHandler(auth, access, settingsUpdateHook{store, func() { after() }})
	if err != nil {
		t.Fatal(err)
	}
	cookie, csrf := launch(t, auth, h)
	otherCookie, _ := launch(t, auth, h)
	browser := map[string]string{"Cookie": cookie, "Origin": "http://localhost:7777", "X-Amaru-CSRF": csrf}
	writes := 0
	after = func() {
		writes++
		if w := call(h, nil, http.MethodDelete, "/v1/auth/session", "", browser); w.Code != http.StatusNoContent {
			t.Fatal("logout during settings write failed")
		}
	}
	w := call(h, nil, http.MethodPatch, path(projectA), `{"importClaudeSkills":true}`, browser)
	check(t, w, http.StatusForbidden, "forbidden")
	if strings.Contains(w.Body.String(), "importClaudeSkills") || writes != 1 {
		t.Fatal("revoked write returned settings or repeated mutation")
	}
	check(t, call(h, nil, http.MethodGet, path(projectA), "", browser), http.StatusUnauthorized, "unauthorized")
	for _, client := range []struct {
		key  domain.ProjectKey
		want string
	}{{projectA, `{"importClaudeSkills":true}`}, {projectB, `{"importClaudeSkills":false}`}} {
		w := call(h, nil, http.MethodGet, path(client.key), "", map[string]string{"Cookie": otherCookie})
		check(t, w, http.StatusOK, "")
		if strings.TrimSpace(w.Body.String()) != client.want {
			t.Fatal("response denial rolled back the commit or affected another Project/session")
		}
	}
}

func (s settingsReadHook) Settings(ctx context.Context, key domain.ProjectKey) (domain.ProjectSettings, error) {
	value, err := s.SettingsStore.Settings(ctx, key)
	s.after()
	return value, err
}

func TestSettingsSessionLogoutDuringRead(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			auth, _, store := localSetup(t)
			ctx := context.Background()
			enabled := true
			if _, err := store.UpdateSettings(ctx, projectA, domain.UpdateSettings{ImportClaudeSkills: &enabled}); err != nil {
				t.Fatal(err)
			}
			access, err := coordinator.NewProjectAccess(coordinator.New(fakes.NewExecutionStore(), time.Now), store.ProjectStore, auth.Principal)
			if err != nil {
				t.Fatal(err)
			}
			after := func() {}
			h, err := SettingsHandler(auth, access, settingsReadHook{store, func() { after() }})
			if err != nil {
				t.Fatal(err)
			}
			cookie, csrf := launch(t, auth, h)
			otherCookie, _ := launch(t, auth, h)
			browser := map[string]string{"Cookie": cookie, "Origin": "http://localhost:7777", "X-Amaru-CSRF": csrf}
			beforeReads, beforeWrites := store.reads, store.writes
			after = func() {
				after = func() {}
				if w := call(h, nil, http.MethodDelete, "/v1/auth/session", "", browser); w.Code != http.StatusNoContent {
					t.Fatal("logout during settings read failed")
				}
			}
			r := httptest.NewRequest(method, "http://localhost:7777"+path(projectA), nil)
			for k, v := range browser {
				r.Header.Set(k, v)
			}
			r.Header.Set("Content-Type", "application/json")
			bodyRead := false
			body := strings.NewReader(`{"importClaudeSkills":false}`)
			r.Body = readerFunc(func(p []byte) (int, error) { bodyRead = true; return body.Read(p) })
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			check(t, w, http.StatusForbidden, "forbidden")
			if strings.Contains(w.Body.String(), "importClaudeSkills") || bodyRead || store.reads != beforeReads+1 || store.writes != beforeWrites {
				t.Fatal("revoked read exposed settings, consumed the patch body or mutated storage")
			}
			beforeReads = store.reads
			check(t, call(h, nil, http.MethodGet, path(projectA), "", browser), http.StatusUnauthorized, "unauthorized")
			if store.reads != beforeReads {
				t.Fatal("logged-out session reached storage")
			}
			for _, client := range []struct {
				key  domain.ProjectKey
				want string
			}{{projectA, `{"importClaudeSkills":true}`}, {projectB, `{"importClaudeSkills":false}`}} {
				w = call(h, nil, http.MethodGet, path(client.key), "", map[string]string{"Cookie": otherCookie})
				check(t, w, http.StatusOK, "")
				if strings.TrimSpace(w.Body.String()) != client.want {
					t.Fatal("other session lost access or Project settings changed")
				}
			}
		})
	}
}
