package coordinator_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
)

type settingsWriteHook struct {
	ports.SettingsStore
	after   func()
	failure error
}

func (s settingsWriteHook) UpdateSettings(ctx context.Context, key domain.ProjectKey, patch domain.UpdateSettings) (domain.ProjectSettings, error) {
	if s.failure != nil {
		s.after()
		return domain.ProjectSettings{}, s.failure
	}
	value, err := s.SettingsStore.UpdateSettings(ctx, key, patch)
	s.after()
	return value, err
}

func TestProjectSettingsWriteAuthorityDuringIO(t *testing.T) {
	failed := errors.New("storage unavailable")
	for _, outcome := range []string{"authorized", "identity changed", "revoked", "canceled", "storage error", "storage error with identity changed"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			projects := fakes.NewProjectStore()
			project := domain.ProjectKey("ssh://github.com/acme/a#0123456789abcdef0123456789abcdef01234567")
			if err := projects.Create(ctx, domain.Project{Key: project, Name: "project", CreatedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			owner := domain.Principal{Kind: domain.PrincipalHuman, ID: "owner", Role: domain.RoleOwner}
			revoked := false
			api, err := coordinator.NewProjectAccess(coordinator.New(fakes.NewExecutionStore(), time.Now), projects, func(context.Context) (domain.Principal, error) {
				if revoked {
					return domain.Principal{}, domain.ErrNotPermitted
				}
				return owner, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			store := settingsWriteHook{SettingsStore: projects, after: func() {
				switch outcome {
				case "identity changed", "storage error with identity changed":
					owner.ID = "replacement-owner"
				case "revoked":
					revoked = true
				case "canceled":
					cancel()
				}
			}}
			var want error
			switch outcome {
			case "identity changed", "revoked":
				want = domain.ErrNotPermitted
			case "canceled":
				want = context.Canceled
			case "storage error", "storage error with identity changed":
				store.failure, want = failed, failed
			}
			enabled := true
			got, err := api.UpdateSettings(ctx, project, domain.UpdateSettings{ImportClaudeSkills: &enabled}, store)
			if !errors.Is(err, want) || got.ImportClaudeSkills != (want == nil) {
				t.Fatalf("result: %+v %v; want success=%v, error=%v", got, err, want == nil, want)
			}
			persisted, err := projects.Settings(context.Background(), project)
			if err != nil || persisted.ImportClaudeSkills != (store.failure == nil) {
				t.Fatalf("committed settings changed after response denial: %+v %v", persisted, err)
			}
		})
	}
}

func TestProjectSettingsAccess(t *testing.T) {
	ctx := context.Background()
	projects := fakes.NewProjectStore()
	a := domain.ProjectKey("ssh://github.com/acme/a#0123456789abcdef0123456789abcdef01234567")
	b := domain.ProjectKey("ssh://github.com/acme/b#0123456789abcdef0123456789abcdef01234567")
	foreign := domain.ProjectKey("ssh://github.com/acme/foreign#0123456789abcdef0123456789abcdef01234567")
	for _, key := range []domain.ProjectKey{a, b} {
		if err := projects.Create(ctx, domain.Project{Key: key, Name: "same", CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	owner := domain.Principal{Kind: domain.PrincipalHuman, ID: "owner", Role: domain.RoleOwner}
	denied := false
	api, err := coordinator.NewProjectAccess(coordinator.New(fakes.NewExecutionStore(), time.Now), projects, func(context.Context) (domain.Principal, error) {
		if denied {
			return domain.Principal{}, domain.ErrNotPermitted
		}
		return owner, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	value := true
	patch := domain.UpdateSettings{ImportClaudeSkills: &value}
	for range 2 {
		got, err := api.UpdateSettings(ctx, a, patch, projects)
		if err != nil || !got.ImportClaudeSkills {
			t.Fatalf("update: %+v %v", got, err)
		}
	}
	for _, key := range []domain.ProjectKey{foreign, "invalid"} {
		if _, err := api.Settings(ctx, key, projects); err != domain.ErrNotFound {
			t.Fatalf("foreign read: %v", err)
		}
		if _, err := api.UpdateSettings(ctx, key, patch, projects); err != domain.ErrNotFound {
			t.Fatalf("foreign write: %v", err)
		}
	}
	if _, err := api.UpdateSettings(ctx, a, domain.UpdateSettings{}, projects); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty: %v", err)
	}
	denied = true
	value = false
	if _, err := api.UpdateSettings(ctx, a, patch, projects); err != domain.ErrNotPermitted {
		t.Fatalf("revoked write: %v", err)
	}
	if _, err := api.Settings(ctx, a, projects); err != domain.ErrNotPermitted {
		t.Fatalf("revoked read: %v", err)
	}
	denied = false
	owner.Role = domain.RoleContributor
	if _, err := api.UpdateSettings(ctx, a, patch, projects); err != domain.ErrNotPermitted {
		t.Fatalf("nonowner: %v", err)
	}
	first, err := projects.Settings(ctx, a)
	if err != nil || !first.ImportClaudeSkills {
		t.Fatalf("rejected mutation: %+v %v", first, err)
	}
	second, err := projects.Settings(ctx, b)
	if err != nil || second.ImportClaudeSkills {
		t.Fatalf("foreign mutation: %+v %v", second, err)
	}
}
