package coordinator_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
)

type workflowSource func(context.Context, domain.ProjectKey, string, string) (domain.Workflow, error)

func (f workflowSource) Workflow(ctx context.Context, project domain.ProjectKey, id, revision string) (ports.ProjectWorkflow, error) {
	w, err := f(ctx, project, id, revision)
	return ports.ProjectWorkflow{Workflow: w, Clone: domain.ProjectClone{ProjectKey: project, Root: "test-clone"}}, err
}

func TestProjectWorkflowAuthorization(t *testing.T) {
	ctx := context.Background()
	project := domain.ProjectKey("ssh://example.com/app#" + strings.Repeat("a", 40))
	projects := fakes.NewProjectStore()
	if err := projects.Create(ctx, domain.Project{Key: project, Name: "App", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	store := fakes.NewExecutionStore()
	c := coordinator.New(store, time.Now)
	revoked := false
	api, err := coordinator.NewProjectAccess(c, projects, func(context.Context) (domain.Principal, error) {
		if revoked {
			return domain.Principal{}, domain.ErrNotPermitted
		}
		return domain.Principal{Kind: domain.PrincipalHuman, ID: "owner", Role: domain.RoleOwner}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	req := coordinator.RunWorkflowRequest{Key: "run", WorkflowID: "same-flow", Revision: strings.Repeat("a", 64)}
	if _, _, err := api.RunWorkflow(ctx, domain.ProjectKey("ssh://example.com/other#"+strings.Repeat("a", 40)), req, nil); err != domain.ErrNotFound {
		t.Fatalf("unattached Project reached source: %v", err)
	}
	w := domain.Workflow{ID: "same-flow", Name: "Same", Status: domain.WorkflowActive, Steps: []domain.WorkflowStep{{ID: "test", Name: "Test", Type: domain.StepTypeTool, Capability: "code.test"}}}
	source := workflowSource(func(_ context.Context, p domain.ProjectKey, id, revision string) (domain.Workflow, error) {
		if p != project || id != req.WorkflowID || revision != req.Revision {
			t.Fatal("source selected wrong content")
		}
		return w, nil
	})
	for _, tc := range []struct {
		name   string
		change func()
	}{
		{"different id", func() { w.ID = "other-flow" }},
		{"invalid definition", func() { w.ID = req.WorkflowID; w.Name = "" }},
		{"draft", func() { w.Name = "Same"; w.Status = domain.WorkflowDraft }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.change()
			out, replay, err := api.RunWorkflow(ctx, project, req, source)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("bad source: %v", err)
			}
			if err == nil || replay || out.Execution.ID != "" {
				t.Fatalf("bad source created Execution: %+v %v %v", out, replay, err)
			}
		})
	}
	w.Status = domain.WorkflowActive
	source = workflowSource(func(context.Context, domain.ProjectKey, string, string) (domain.Workflow, error) {
		revoked = true
		return w, nil
	})
	if got, err := api.Workflow(ctx, project, req.WorkflowID, req.Revision, source); err != domain.ErrNotPermitted || got.ID != "" {
		t.Fatalf("session revoked during load leaked Workflow: %+v %v", got, err)
	}
	revoked = false
	if _, _, err := api.RunWorkflow(ctx, project, req, source); err != domain.ErrNotPermitted {
		t.Fatalf("session revoked during load reached Submit: %v", err)
	}
}

type selectedWorkflowSource struct{ selected ports.ProjectWorkflow }

func (s selectedWorkflowSource) Workflow(context.Context, domain.ProjectKey, string, string) (ports.ProjectWorkflow, error) {
	return s.selected, nil
}

func TestWorkflowRejectsForeignCloneProvenance(t *testing.T) {
	ctx := context.Background()
	project := domain.ProjectKey("ssh://example.com/app#" + strings.Repeat("a", 40))
	projects := fakes.NewProjectStore()
	if err := projects.Create(ctx, domain.Project{Key: project, Name: "App", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	api, err := coordinator.NewProjectAccess(coordinator.New(fakes.NewExecutionStore(), time.Now), projects, func(context.Context) (domain.Principal, error) {
		return domain.Principal{Kind: domain.PrincipalHuman, ID: "owner", Role: domain.RoleOwner}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	w := domain.Workflow{ID: "flow", Name: "Flow", Status: domain.WorkflowActive, Steps: []domain.WorkflowStep{{ID: "test", Name: "Test", Type: domain.StepTypeTool, Capability: "code.test"}}}
	for _, clone := range []domain.ProjectClone{{}, {ProjectKey: project}, {ProjectKey: project, Root: "bad\x00root"}, {ProjectKey: "other", Root: t.TempDir()}} {
		source := selectedWorkflowSource{ports.ProjectWorkflow{Workflow: w, Clone: clone}}
		if got, err := api.Workflow(ctx, project, "flow", strings.Repeat("a", 40), source); err != domain.ErrInvalid || got.ID != "" {
			t.Fatalf("foreign/invalid source provenance: %+v %v", got, err)
		}
	}
}

func TestWorkflowPreservesIdentity(t *testing.T) {
	project := domain.ProjectKey("ssh://example.com/app#" + strings.Repeat("a", 40))
	for _, operation := range []string{"read", "submit", "replay"} {
		for _, outcome := range []string{"authorized", "identity changed", "revoked", "canceled", "source error", "source error and revoked", "source error and changed identity", "identity before submit"} {
			if operation == "read" && outcome == "identity before submit" {
				continue
			}
			t.Run(operation+"/"+outcome, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				owner := domain.Principal{Kind: domain.PrincipalHuman, ID: "original-owner", Role: domain.RoleOwner}
				current, revoked, calls := owner, false, 0
				projects := fakes.NewProjectStore()
				if err := projects.Create(ctx, domain.Project{Key: project, Name: "App", CreatedAt: time.Now()}); err != nil {
					t.Fatal(err)
				}
				store := &mutationAuthorityStore{ExecutionStore: fakes.NewExecutionStore()}
				armed := false
				api, err := coordinator.NewProjectAccess(coordinator.New(store, time.Now), projects, func(context.Context) (domain.Principal, error) {
					calls++
					// Two checks per Project lookup; the fifth begins submission authorization.
					if armed && outcome == "identity before submit" && calls >= 5 {
						current.ID = "replacement-owner"
					}
					if revoked {
						return domain.Principal{}, domain.ErrNotPermitted
					}
					return current, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				w := domain.Workflow{ID: "flow", Name: "Flow", Status: domain.WorkflowActive, Steps: []domain.WorkflowStep{{ID: "test", Name: "Test", Type: domain.StepTypeTool, Capability: "code.test"}}}
				req := coordinator.RunWorkflowRequest{Key: "run", WorkflowID: w.ID, Revision: strings.Repeat("a", 40)}
				failed := errors.New("source unavailable")
				source := workflowSource(func(context.Context, domain.ProjectKey, string, string) (domain.Workflow, error) {
					if armed {
						switch outcome {
						case "identity changed", "source error and changed identity":
							current.ID = "replacement-owner"
						case "revoked", "source error and revoked":
							revoked = true
						case "canceled":
							cancel()
						}
						if strings.HasPrefix(outcome, "source error") {
							return w, failed
						}
					}
					return w, nil
				})
				if operation == "replay" {
					if _, replay, err := api.RunWorkflow(ctx, project, req, source); err != nil || replay {
						t.Fatalf("seed: %v %v", replay, err)
					}
				}
				commands, commits := store.commands.Load(), store.commits.Load()
				calls, armed = 0, true
				var got, zero any
				replay := false
				if operation == "read" {
					got, err = api.Workflow(ctx, project, req.WorkflowID, req.Revision, source)
					zero = domain.Workflow{}
				} else {
					var out coordinator.Submitted
					out, replay, err = api.RunWorkflow(ctx, project, req, source)
					got, zero = out, coordinator.Submitted{}
					if outcome == "authorized" && out.Execution.RequestedBy != owner {
						t.Fatalf("actor: %+v", out.Execution.RequestedBy)
					}
				}
				if outcome == "authorized" {
					if err != nil || reflect.DeepEqual(got, zero) || replay != (operation == "replay") {
						t.Fatalf("positive control: %+v %v replay=%v", got, err, replay)
					}
					return
				}
				want := domain.ErrNotPermitted
				if outcome == "canceled" {
					want = context.Canceled
				}
				if outcome == "source error" {
					want = failed
				}
				if !errors.Is(err, want) || !reflect.DeepEqual(got, zero) || replay {
					t.Fatalf("want empty and %v; got %+v %v replay=%v", want, got, err, replay)
				}
				if store.commands.Load() != commands || store.commits.Load() != commits {
					t.Fatal("denied operation reached command replay or mutation")
				}
			})
		}
	}
}
