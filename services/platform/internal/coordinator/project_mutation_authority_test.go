package coordinator_test

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/coordinator/coordtest"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
)

// Pause a successful resource read before the application boundary receives it.
type mutationAuthorityStore struct {
	ports.ExecutionStore
	stage             string
	armed             atomic.Bool
	entered, release  chan struct{}
	failure           error
	foreign           bool
	commands, commits atomic.Int64
}

func (s *mutationAuthorityStore) pause(stage string) bool {
	if s.stage != stage || !s.armed.CompareAndSwap(true, false) {
		return false
	}
	close(s.entered)
	<-s.release
	return true
}

func (s *mutationAuthorityStore) Get(ctx context.Context, id string) (domain.Execution, []domain.StepExecution, error) {
	e, steps, err := s.ExecutionStore.Get(ctx, id)
	if s.pause("execution") {
		if s.foreign {
			e.ProjectID = "foreign"
		}
		if s.failure != nil {
			err = s.failure
		}
	}
	return e, steps, err
}

func (s *mutationAuthorityStore) Approval(ctx context.Context, id string) (domain.Approval, error) {
	a, err := s.ExecutionStore.Approval(ctx, id)
	if s.pause("approval") {
		if s.foreign {
			a.ProjectID = "foreign"
		}
		if s.failure != nil {
			err = s.failure
		}
	}
	return a, err
}

func (s *mutationAuthorityStore) Command(ctx context.Context, name, key string) (domain.CommandRecord, error) {
	s.commands.Add(1)
	return s.ExecutionStore.Command(ctx, name, key)
}

func (s *mutationAuthorityStore) Commit(ctx context.Context, change ports.ExecutionChange) (ports.ExecutionChange, error) {
	s.commits.Add(1)
	return s.ExecutionStore.Commit(ctx, change)
}

func TestProjectMutationAuthorityDuringIO(t *testing.T) {
	for _, operation := range []string{"cancel", "decision/approval", "decision/execution", "resume", "abandon"} {
		for _, phase := range []string{"mutation", "replay"} {
			for _, outcome := range []string{"authorized", "revoked", "canceled", "identity changed", "storage error", "missing", "foreign"} {
				t.Run(operation+"/"+phase+"/"+outcome, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					project := domain.ProjectKey("ssh://github.com/acme/a#0123456789abcdef0123456789abcdef01234567")
					owner := domain.Principal{Kind: domain.PrincipalHuman, ID: "original-owner", Role: domain.RoleOwner}
					projects := fakes.NewProjectStore()
					if err := projects.Create(ctx, domain.Project{Key: project, Name: "project", CreatedAt: time.Now()}); err != nil {
						t.Fatal(err)
					}
					base := fakes.NewExecutionStore()
					store := &mutationAuthorityStore{ExecutionStore: base, stage: "execution", entered: make(chan struct{}), release: make(chan struct{})}
					if operation == "decision/approval" {
						store.stage = "approval"
					}
					c := coordinator.New(store, func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }, coordtest.NewOpenTrust().Option())
					var revoked, changed atomic.Bool
					api, err := coordinator.NewProjectAccess(c, projects, func(context.Context) (domain.Principal, error) {
						if revoked.Load() {
							return domain.Principal{}, domain.ErrNotPermitted
						}
						by := owner
						if changed.Load() {
							by.ID = "replacement-owner"
						}
						return by, nil
					})
					if err != nil {
						t.Fatal(err)
					}
					execution, step, approval, digest := "execution", "step", "", ""
					if operation == "decision/approval" || operation == "decision/execution" {
						submitted, _, err := c.Submit(ctx, coordinator.SubmitRequest{Key: "submit", ProjectID: string(project), By: owner, Steps: []coordinator.StepSpec{{Name: "push", Capability: "git.push"}}})
						if err != nil || len(submitted.Approvals) != 1 {
							t.Fatalf("seed approval: %v", err)
						}
						execution, step = submitted.Execution.ID, submitted.Steps[0].ID
						approval, digest = submitted.Approvals[0].ID, submitted.Approvals[0].Evidence.Digest
					} else {
						_, _, err := base.Create(ctx, domain.Execution{ID: execution, ProjectID: string(project), State: domain.ExecRunning, RequestedBy: owner}, []domain.StepExecution{{ID: step, ExecutionID: execution, State: domain.StepInterrupted, Capability: "code.test"}})
						if err != nil {
							t.Fatal(err)
						}
					}
					var zero any
					call := func() (any, bool, error) {
						switch operation {
						case "cancel":
							return api.Cancel(ctx, project, coordinator.CancelRequest{Key: "mutation", ExecutionID: execution, By: domain.Actor{Kind: domain.ActorPolicy}, ActorID: "spoof"})
						case "decision/approval", "decision/execution":
							return api.DecideApproval(ctx, project, coordinator.DecideRequest{Key: "mutation", ApprovalID: approval, EvidenceDigest: digest, By: domain.Principal{ID: "spoof"}, Outcome: domain.OutcomeApproved})
						default:
							req := coordinator.HumanStepRequest{Key: "mutation", ExecutionID: execution, StepID: step, By: domain.Actor{Kind: domain.ActorPolicy}, ActorID: "spoof"}
							if operation == "resume" {
								return api.ResumeStep(ctx, project, req)
							}
							return api.AbandonStep(ctx, project, req)
						}
					}
					switch operation {
					case "cancel":
						zero = coordinator.Cancelled{}
					case "decision/approval", "decision/execution":
						zero = coordinator.Decided{}
					default:
						zero = domain.StepExecution{}
					}
					var original any
					if phase == "replay" {
						var replay bool
						original, replay, err = call()
						if err != nil || replay {
							t.Fatalf("seed mutation: %v replay=%v", err, replay)
						}
					}
					commands, commits := store.commands.Load(), store.commits.Load()
					failed := errors.New("storage unavailable")
					switch outcome {
					case "storage error":
						store.failure = failed
					case "missing":
						store.failure = domain.ErrNotFound
					case "foreign":
						store.foreign = true
					}
					store.armed.Store(true)
					type result struct {
						out    any
						replay bool
						err    error
					}
					done := make(chan result, 1)
					go func() { out, replay, err := call(); done <- result{out, replay, err} }()
					select {
					case <-store.entered:
					case <-time.After(5 * time.Second):
						t.Fatal("resource read was not reached")
					}
					switch outcome {
					case "revoked":
						revoked.Store(true)
					case "canceled":
						cancel()
					case "identity changed":
						changed.Store(true)
					case "storage error", "missing", "foreign":
						revoked.Store(true) // resource errors keep precedence
					}
					close(store.release)
					var got result
					select {
					case got = <-done:
					case <-time.After(5 * time.Second):
						t.Fatal("command did not return")
					}
					if outcome == "authorized" {
						if got.err != nil || got.replay != (phase == "replay") || reflect.DeepEqual(got.out, zero) {
							t.Fatalf("authorized command: %+v", got)
						}
						if phase == "replay" && !reflect.DeepEqual(got.out, original) {
							t.Fatal("replay outcome changed")
						}
						// Replay verifies the original actor in the durable fingerprint.
						again, replay, err := call()
						if err != nil || !replay || !reflect.DeepEqual(again, got.out) {
							t.Fatalf("original actor lost: %v replay=%v", err, replay)
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
					if !errors.Is(got.err, want) || got.replay || !reflect.DeepEqual(got.out, zero) {
						t.Fatalf("want empty result, no replay and %v; got %+v", want, got)
					}
					if store.commands.Load() != commands || store.commits.Load() != commits {
						t.Fatal("rejected request reached coordinator replay or mutation")
					}
				})
			}
		}
	}
}
