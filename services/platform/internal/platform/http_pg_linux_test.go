//go:build pg && linux

package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rluders/amaru/services/platform/internal/adapter/discovery"
	"github.com/rluders/amaru/services/platform/internal/adapter/httpapi"
	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/store/postgres"
	"github.com/rluders/lane/v2"
)

func TestPostgresHTTPWriteDrainsBeforeStorageCloses(t *testing.T) {
	config := lifecycleDatabase(t)
	observer := sql.OpenDB(stdlib.GetConnector(*config))
	defer observer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := filepath.Join(t.TempDir(), "state")
	key := domain.ProjectKey("id:01ARZ3NDEKTSV4RRFFQ69G5FAV")
	path := "/v1/projects/" + string(key) + "/settings"
	var oldSecret string
	for round := 0; round < 2; round++ {
		func() {
			deadline := time.Now().Add(10 * time.Second)
			storage, _ := NewStorage(stdlib.GetConnector(*config), deadline)
			schema, _ := NewSchema(storage)
			recovery, _ := NewRecovery(storage)
			lifecycle, _ := NewLifecycle(deadline, 3*time.Second, nil)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			h, _ := httpOwner(t, dir, lifecycle, func(auth *localauth.Auth) (http.Handler, error) {
				db, err := storage.DB(ctx)
				if err != nil {
					return nil, err
				}
				c, err := recovery.Coordinator(ctx)
				if err != nil {
					return nil, err
				}
				projects := postgres.NewProjectStore(db)
				if round == 0 {
					if err := projects.Create(ctx, domain.Project{Key: key, Name: "retained", CreatedAt: time.Now().UTC()}); err != nil {
						return nil, err
					}
				}
				access, err := coordinator.NewProjectAccess(c, projects, auth.Principal)
				if err != nil {
					return nil, err
				}
				handler, err := httpapi.SettingsHandler(auth, access, projects)
				if err != nil {
					return nil, err
				}
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPatch {
						close(entered)
						select {
						case <-release:
						case <-r.Context().Done():
							t.Error("ordinary write cancelled before drain")
							return
						}
					}
					handler.ServeHTTP(w, r)
				}), nil
			})
			process, stop := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() {
				done <- lifecycle.Run(process, []lane.Runner{storage}, []lane.Runner{schema}, []lane.Runner{recovery}, []lane.Runner{h})
			}()
			joined := false
			defer func() {
				stop()
				unblock()
				if !joined {
					if err := result(t, done); err != nil {
						t.Error("lifecycle cleanup failed")
					}
				}
			}()
			info := readyHTTP(t, h)
			native := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+info.AdministratorSecret) }
			read := func() {
				resp := httpCall(t, info, "GET", path, "", native)
				defer resp.Body.Close()
				var settings domain.ProjectSettings
				if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&settings) != nil || settings.ImportClaudeSkills != (round == 1) {
					t.Fatal("HTTP read did not observe durable Project settings")
				}
			}
			read()
			if round == 0 {
				oldSecret = info.AdministratorSecret
				writeDone := make(chan error, 1)
				go func() {
					req, err := http.NewRequestWithContext(ctx, "PATCH", info.Endpoint+path, strings.NewReader(`{"importClaudeSkills":true}`))
					if err != nil {
						writeDone <- errors.New("request creation failed")
						return
					}
					native(req)
					req.Header.Set("Content-Type", "application/json")
					client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 5 * time.Second}
					resp, err := client.Do(req)
					if err != nil {
						writeDone <- errors.New("draining write failed")
						return
					}
					defer resp.Body.Close()
					var settings domain.ProjectSettings
					if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&settings) != nil || !settings.ImportClaudeSkills {
						writeDone <- errors.New("draining write did not commit")
						return
					}
					writeDone <- nil
				}()
				defer func() {
					unblock()
					if err := result(t, writeDone); err != nil {
						t.Error(err)
					}
				}()
				wait(t, entered)
				stop()
				for lifecycle.Ready() {
					if ctx.Err() != nil {
						t.Fatal("admission remained open")
					}
					time.Sleep(time.Millisecond)
				}
				w := httptest.NewRecorder()
				h.server.Handler.ServeHTTP(w, httptest.NewRequest("GET", info.Endpoint+path, nil))
				if w.Code != http.StatusServiceUnavailable {
					t.Fatal("shutdown admitted another request")
				}
				select {
				case <-done:
					joined = true
					t.Fatal("lifecycle returned before HTTP write joined")
				default:
				}
				if err := storage.db.PingContext(ctx); err != nil {
					t.Fatal("pool closed during HTTP drain")
				}
				if _, err := discovery.Read(dir); err != nil {
					t.Fatal("publication removed before HTTP drain")
				}
				unblock()
			} else {
				if info.AdministratorSecret == oldSecret {
					t.Fatal("fresh owner reused administrator credential")
				}
				resp := httpCall(t, info, "GET", path, "", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+oldSecret) })
				resp.Body.Close()
				if resp.StatusCode != http.StatusUnauthorized {
					t.Fatal("fresh owner accepted stale credential")
				}
				stop()
			}
			joined = true
			if err := result(t, done); err != nil {
				t.Fatal("shutdown failed")
			}
			if err := storage.db.PingContext(ctx); err == nil {
				t.Fatal("owned pool remained open")
			}
			if h.auth.AdministratorSecret() != "" {
				t.Fatal("Auth retained credential after join")
			}
			if _, err := os.Stat(filepath.Join(dir, discovery.FileName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("discovery retained after shutdown")
			}
			settings, err := postgres.NewProjectStore(observer).Settings(ctx, key)
			if err != nil || !settings.ImportClaudeSkills {
				t.Fatal("shutdown lost committed settings")
			}
		}()
	}
}
