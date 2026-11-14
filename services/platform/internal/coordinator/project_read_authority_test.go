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
