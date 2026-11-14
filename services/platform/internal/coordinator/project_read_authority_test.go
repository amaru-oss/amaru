package coordinator_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
)

type readAuthorityStore struct {
	ports.ExecutionStore
	after   func()
	failure error
	project domain.ProjectKey
}

func (s readAuthorityStore) Get(context.Context, string) (domain.Execution, []domain.StepExecution, error) {
	s.after()
	return domain.Execution{ID: "execution", ProjectID: string(s.project)}, []domain.StepExecution{{ID: "step", ExecutionID: "execution"}}, s.failure
}
func (s readAuthorityStore) Approval(context.Context, string) (domain.Approval, error) {
	s.after()
	return domain.Approval{ID: "approval", ProjectID: string(s.project), Gate: domain.ApprovalGate{ExecutionID: "execution"}}, s.failure
}

type readAuthorityProjects struct {
	ports.ProjectStore
	after func()
}

func (s readAuthorityProjects) Get(ctx context.Context, key domain.ProjectKey) (domain.Project, error) {
	p, err := s.ProjectStore.Get(ctx, key)
	s.after()
	return p, err
}

type readAuthorityStream struct {
	after   func()
	failure error
}

func (s readAuthorityStream) ReadEvents(context.Context, domain.EventQuery, time.Time, time.Duration) (domain.EventPage, error) {
	s.after()
	return domain.EventPage{Cursor: 42}, s.failure
}

type readAuthorityEvidence struct {
	after   func()
	failure error
}

func (s readAuthorityEvidence) Bundle(context.Context, string) (domain.ExecutionEvidence, error) {
	s.after()
	return domain.ExecutionEvidence{Digest: "private"}, s.failure
}

type readAuthoritySettings struct {
	ports.SettingsStore
	after   func()
	failure error
}

func (s readAuthoritySettings) Settings(context.Context, domain.ProjectKey) (domain.ProjectSettings, error) {
	s.after()
	return domain.ProjectSettings{ImportClaudeSkills: true}, s.failure
}

func TestProjectReadAuthorityDuringIO(t *testing.T) {
	project := domain.ProjectKey("ssh://github.com/acme/a#0123456789abcdef0123456789abcdef01234567")
	owner := domain.Principal{Kind: domain.PrincipalHuman, ID: "owner", Role: domain.RoleOwner}
	failed := errors.New("storage unavailable")
	for _, operation := range []string{"project lookup", "execution", "approval", "events", "evidence", "settings"} {
		for _, outcome := range []string{"authorized", "revoked", "canceled", "storage error"} {
			if operation == "project lookup" && outcome == "storage error" {
				continue
			}
			t.Run(operation+"/"+outcome, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				revoked := false
				after := func() {
					if outcome == "revoked" {
						revoked = true
					}
					if outcome == "canceled" {
						cancel()
					}
				}
				var failure error
				if outcome == "storage error" {
					failure = failed
				}
				projects := fakes.NewProjectStore()
				if err := projects.Create(ctx, domain.Project{Key: project, Name: "project", CreatedAt: time.Now()}); err != nil {
					t.Fatal(err)
				}
				store := readAuthorityStore{after: func() {}, project: project}
				var projectStore ports.ProjectStore = projects
				if operation == "project lookup" {
					projectStore = readAuthorityProjects{projects, after}
				}
				if operation == "execution" || operation == "approval" {
					store.after, store.failure = after, failure
				}
				api, err := coordinator.NewProjectAccess(coordinator.New(store, time.Now), projectStore, func(context.Context) (domain.Principal, error) {
					if revoked {
						return domain.Principal{}, domain.ErrNotPermitted
					}
					return owner, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				var got, zero any
				switch operation {
				case "project lookup", "execution":
					var e domain.Execution
					var steps []domain.StepExecution
					e, steps, err = api.Execution(ctx, project, "execution")
					got, zero = []any{e, steps}, []any{domain.Execution{}, []domain.StepExecution(nil)}
				case "approval":
					got, err = api.Approval(ctx, project, "approval")
					zero = domain.Approval{}
				case "events":
					got, err = api.Events(ctx, domain.EventQuery{ProjectID: string(project), Limit: 10}, readAuthorityStream{after, failure}, time.Hour)
					zero = domain.EventPage{}
				case "evidence":
					got, err = api.Evidence(ctx, project, "execution", readAuthorityEvidence{after, failure})
					zero = domain.ExecutionEvidence{}
				case "settings":
					got, err = api.Settings(ctx, project, readAuthoritySettings{after: after, failure: failure})
					zero = domain.ProjectSettings{}
				}
				if outcome == "authorized" {
					if err != nil || reflect.DeepEqual(got, zero) {
						t.Fatalf("authorized read withheld: %v", err)
					}
					return
				}
				want := failed
				if outcome == "revoked" {
					want = domain.ErrNotPermitted
				}
				if outcome == "canceled" {
					want = context.Canceled
				}
				if !errors.Is(err, want) || !reflect.DeepEqual(got, zero) {
					t.Fatalf("read must return no data and %v; got error %v, empty=%v", want, err, reflect.DeepEqual(got, zero))
				}
			})
		}
	}
}

// Project lookup must not replace the actor before command attribution or resource access.
func TestProjectLookupPreservesIdentity(t *testing.T) {
	project := domain.ProjectKey("ssh://github.com/acme/a#0123456789abcdef0123456789abcdef01234567")
	for _, operation := range []string{"read", "submit"} {
		for _, outcome := range []string{"authorized", "identity changed", "revoked", "canceled", "storage error", "missing", "foreign"} {
			t.Run(operation+"/"+outcome, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				owner := domain.Principal{Kind: domain.PrincipalHuman, ID: "original-owner", Role: domain.RoleOwner}
				current := owner
				revoked := false
				projects := fakes.NewProjectStore()
				if err := projects.Create(ctx, domain.Project{Key: project, Name: "project", CreatedAt: time.Now()}); err != nil {
					t.Fatal(err)
				}
				failed := errors.New("storage unavailable")
				lookup := lookupIdentityProjects{ProjectStore: projects, after: func(p domain.Project) (domain.Project, error) {
					switch outcome {
					case "identity changed":
						current.ID = "replacement-owner"
					case "revoked":
						revoked = true
					case "canceled":
						cancel()
					case "storage error":
						revoked = true
						return p, failed
					case "missing":
						revoked = true
						return p, domain.ErrNotFound
					case "foreign":
						revoked = true
						p.Key = "foreign"
					}
					return p, nil
				}}
				base := fakes.NewExecutionStore()
				store := &mutationAuthorityStore{ExecutionStore: base}
				reads := 0
				var executionStore ports.ExecutionStore = store
				if operation == "read" {
					executionStore = readAuthorityStore{project: project, after: func() { reads++ }}
				}
				api, err := coordinator.NewProjectAccess(coordinator.New(executionStore, time.Now), lookup, func(context.Context) (domain.Principal, error) {
					if revoked {
						return domain.Principal{}, domain.ErrNotPermitted
					}
					return current, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				var got, zero any
				replay := false
				if operation == "read" {
					var e domain.Execution
					var steps []domain.StepExecution
					e, steps, err = api.Execution(ctx, project, "execution")
					got, zero = []any{e, steps}, []any{domain.Execution{}, []domain.StepExecution(nil)}
				} else {
					var out coordinator.Submitted
					out, replay, err = api.Submit(ctx, project, coordinator.SubmitRequest{Key: "submit", Steps: []coordinator.StepSpec{{Name: "test", Capability: "code.test"}}})
					got, zero = out, coordinator.Submitted{}
					if outcome == "authorized" && out.Execution.RequestedBy != owner {
						t.Fatalf("actor changed: %+v", out.Execution.RequestedBy)
					}
				}
				if outcome == "authorized" {
					if err != nil || reflect.DeepEqual(got, zero) || replay {
						t.Fatalf("authorized operation: %+v %v replay=%v", got, err, replay)
					}
					return
				}
				want := domain.ErrNotPermitted
				switch outcome {
				case "canceled":
					want = context.Canceled
				case "storage error":
					want = failed
				case "missing", "foreign":
					want = domain.ErrNotFound
				}
				if !errors.Is(err, want) || !reflect.DeepEqual(got, zero) || replay {
					t.Fatalf("want empty outcome and %v; got %+v %v replay=%v", want, got, err, replay)
				}
				if reads != 0 || store.commands.Load() != 0 || store.commits.Load() != 0 {
					t.Fatal("rejected lookup reached resource read, replay or mutation")
				}
			})
		}
	}
}

type lookupIdentityProjects struct {
	ports.ProjectStore
	after func(domain.Project) (domain.Project, error)
}

func (s lookupIdentityProjects) Get(ctx context.Context, key domain.ProjectKey) (domain.Project, error) {
	p, err := s.ProjectStore.Get(ctx, key)
	if err != nil {
		return p, err
	}
	return s.after(p)
}
