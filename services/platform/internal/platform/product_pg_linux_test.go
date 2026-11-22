//go:build pg && linux

package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rluders/amaru/services/platform/internal/adapter/httpapi"
	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/coordinator/coordtest"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/store/postgres"
	"github.com/rluders/lane/v2"
)

// productClones answers inspection for the registered test roots only.
type productClones map[string]domain.ProjectKey

func (c productClones) Inspect(_ context.Context, root string) (ports.ProjectInspection, error) {
	key, ok := c[root]
	if !ok {
		return ports.ProjectInspection{}, domain.ErrInvalid
	}
	return ports.ProjectInspection{Key: key, Root: root}, nil
}

// TestPostgresProductHandlerServesDurableState composes Storage, Schema, Recovery and HTTP with
// the assembled product handler over PostgreSQL and real loopback. Two Projects are isolated; a
// cancel commits through HTTP; after fresh owners restart on the same database the Execution
// and its Event page, including the derived Event ids, read back identically.
func TestPostgresProductHandlerServesDurableState(t *testing.T) {
	config := lifecycleDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dir := filepath.Join(t.TempDir(), "state")
	keyA, keyB := domain.ProjectKey("id:01ARZ3NDEKTSV4RRFFQ69G5FAV"), domain.ProjectKey("id:01ARZ3NDEKTSV4RRFFQ69G5FAW")
	clones := productClones{"/work/a": keyA, "/work/b": keyB}
	owner := domain.Principal{Kind: domain.PrincipalHuman, ID: "local-administrator", Role: domain.RoleOwner}
	base := func(key domain.ProjectKey) string { return "/v1/projects/" + url.PathEscape(string(key)) }
	var execA, execB, eventsA, executionA string
	for round := 0; round < 2; round++ {
		func() {
			deadline := time.Now().Add(15 * time.Second)
			storage, _ := NewStorage(stdlib.GetConnector(*config), deadline)
			schema, _ := NewSchema(storage)
			// Default Policy refuses Steps of untrusted Projects, and this composition wires no
			// trust store; the permissive test rules let a Step stay queued for the cancel.
			recovery, _ := NewRecovery(storage, coordtest.Permissive)
			lifecycle, _ := NewLifecycle(deadline, 8*time.Second, nil)
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
					seed, err := coordinator.NewProjectAccess(c, projects, func(context.Context) (domain.Principal, error) { return owner, nil })
					if err != nil {
						return nil, err
					}
					for root, key := range clones {
						if _, _, err := projects.RegisterClone(ctx, domain.Project{Key: key, Name: root, CreatedAt: time.Now().UTC()}, domain.ProjectClone{ProjectKey: key, Root: root}); err != nil {
							return nil, err
						}
						if _, _, err := seed.Submit(ctx, key, coordinator.SubmitRequest{Key: "seed-" + string(key), Steps: []coordinator.StepSpec{{Name: "test", Capability: "code.test"}}}); err != nil {
							t.Log("seed submit failed:", err)
							return nil, err
						}
					}
				}
				return httpapi.ProductHandler(auth, httpapi.ProductConfig{
					Coordinator: c, Projects: projects, Inspector: clones,
					Events: postgres.NewExecutionStore(db), ReplayWindow: domain.CompactReplayWindow,
				})
			})
			process, stop := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() {
				done <- lifecycle.Run(process, []lane.Runner{storage}, []lane.Runner{schema}, []lane.Runner{recovery}, []lane.Runner{h})
			}()
			defer func() {
				stop()
				if err := result(t, done); err != nil {
					t.Error("lifecycle cleanup failed")
				}
			}()
			info := readyHTTP(t, h)
			native := func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+info.AdministratorSecret)
				if r.Method == http.MethodPost {
					r.Header.Set("Content-Type", "application/json")
				}
			}
			get := func(path string, status int) string {
				t.Helper()
				resp := httpCall(t, info, "GET", path, "", native)
				body, _ := io.ReadAll(resp.Body)
				if resp.StatusCode != status {
					t.Fatalf("GET %s: %d %s", path, resp.StatusCode, body)
				}
				return string(body)
			}
			only := func(key domain.ProjectKey) string {
				t.Helper()
				var list []struct{ ID, ProjectID, State string }
				if err := json.Unmarshal([]byte(get(base(key)+"/executions", 200)), &list); err != nil || len(list) != 1 || list[0].ProjectID != string(key) {
					t.Fatalf("Project %s executions: %+v", key, list)
				}
				return list[0].ID
			}
			if round == 0 {
				var projects []struct{ ID string }
				if err := json.Unmarshal([]byte(get("/v1/projects", 200)), &projects); err != nil || len(projects) != 2 {
					t.Fatalf("projects: %+v", projects)
				}
				execA, execB = only(keyA), only(keyB)
				// One Project cannot address the other's Execution or its Events.
				get(base(keyB)+"/executions/"+execA, 404)
				get(base(keyB)+"/executions/"+execA+"/events", 404)
				resp := httpCall(t, info, "POST", base(keyA)+"/executions/"+execA+"/cancel", `{"idempotencyKey":"cancel-1"}`, native)
				if body, _ := io.ReadAll(resp.Body); resp.StatusCode != 200 {
					t.Fatalf("cancel: %d %s", resp.StatusCode, body)
				}
				eventsA = get(base(keyA)+"/executions/"+execA+"/events", 200)
				var page struct {
					Events []struct {
						ID, Type  string
						ProjectID string
					}
					NextCursor *int64
				}
				if err := json.Unmarshal([]byte(eventsA), &page); err != nil || len(page.Events) == 0 || page.Events[len(page.Events)-1].Type != "execution.cancelled" || page.NextCursor == nil {
					t.Fatalf("events after cancel: %s", eventsA)
				}
				if b := get(base(keyB)+"/executions/"+execB+"/events", 200); b != `{"events":[],"nextCursor":0}`+"\n" {
					t.Fatalf("other Project's Events changed: %s", b)
				}
				executionA = get(base(keyA)+"/executions/"+execA, 200)
				get(base(keyA)+"/approvals", 200)
				get(base(keyA)+"/settings", 200)
				if b := get(base(keyA)+"/workflows", 404); b != `{"error":{"code":"not_found","message":"Request rejected."}}`+"\n" {
					t.Fatalf("unserved route: %s", b)
				}
				return
			}
			// Fresh owners over the same database answer the same durable state.
			if only(keyA) != execA || only(keyB) != execB {
				t.Fatal("Execution ids changed across restart")
			}
			if got := get(base(keyA)+"/executions/"+execA+"/events", 200); got != eventsA {
				t.Fatalf("Event page changed across restart:\n%s\n%s", eventsA, got)
			}
			if got := get(base(keyA)+"/executions/"+execA, 200); got != executionA {
				t.Fatalf("Execution changed across restart:\n%s\n%s", executionA, got)
			}
		}()
	}
}
