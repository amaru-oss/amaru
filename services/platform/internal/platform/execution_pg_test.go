//go:build pg

package platform

import (
	"context"
	"database/sql"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rluders/amaru/services/platform/internal/coordinator/coordtest"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/store/postgres"
	"github.com/rluders/amaru/services/platform/internal/worker/embedded"
	"github.com/rluders/lane/v2"
)

func TestPostgresExecutionShutdownAndRecovery(t *testing.T) {
	for _, idempotent := range []bool{false, true} {
		name := "ordinary"
		if idempotent {
			name = "idempotent"
		}
		t.Run(name, func(t *testing.T) {
			config := lifecycleDatabase(t)
			observer := sql.OpenDB(stdlib.GetConnector(*config))
			defer observer.Close()
			store := postgres.NewExecutionStore(observer)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			assignments := make(chan ports.Assignment, 2)
			var firstAssignment ports.Assignment
			var stoppedEvents []domain.ExecutionEvent
			var recovered domain.Execution
			var recoveredSteps []domain.StepExecution
			var recoveredEvents []domain.ExecutionEvent
			for round := 0; round < 3; round++ {
				func() {
					deadline := time.Now().Add(10 * time.Second)
					storage, err := NewStorage(stdlib.GetConnector(*config), deadline)
					if err != nil {
						t.Fatal(err)
					}
					schema, _ := NewSchema(storage)
					recovery, _ := NewRecovery(storage, coordtest.Permissive)
					lifecycle, _ := NewLifecycle(deadline, 5*time.Second, nil)
					process, stop := context.WithCancel(ctx)
					defer stop()
					cancelled, release := make(chan struct{}), make(chan struct{})
					var releaseOnce sync.Once
					unblock := func() { releaseOnce.Do(func() { close(release) }) }
					defer unblock()
					stages := [][]lane.Runner{{storage}, {schema}, {recovery}, {dependent{ready: func(ctx context.Context) error {
						if round == 0 {
							e := domain.Execution{ID: "shutdown", State: domain.ExecQueued, CreatedAt: time.Now().UTC(), RequestedBy: coordtest.Requester, Trigger: domain.TriggerManual}
							_, _, err := store.Create(ctx, e, []domain.StepExecution{{ID: "step", ExecutionID: e.ID, State: domain.StepQueued, Capability: "code.test", InputsJSON: []byte(`{}`), Idempotent: idempotent}})
							return err
						}
						e, steps, err := store.Get(ctx, "shutdown")
						if err != nil {
							return err
						}
						events, err := store.Events(ctx, e.ID)
						if err != nil {
							return err
						}
						wantStep, wantExecution := domain.StepInterrupted, domain.ExecWaiting
						if idempotent {
							wantStep, wantExecution = domain.StepQueued, domain.ExecRunning
						}
						if len(steps) != 1 || steps[0].State != wantStep || steps[0].CurrentLease != nil || e.State != wantExecution {
							t.Errorf("round %d admitted before recovery: Execution=%s Steps=%+v", round, e.State, steps)
						}
						if len(events) <= len(stoppedEvents) {
							t.Error("recovery added no durable Event")
						}
						if round == 1 {
							recovered, recoveredSteps, recoveredEvents = e, steps, events
						} else if !reflect.DeepEqual(recovered, e) || !reflect.DeepEqual(recoveredSteps, steps) || !reflect.DeepEqual(recoveredEvents, events) {
							t.Error("repeated recovery changed settled state or Events")
						}
						return nil
					}}}}
					if round == 0 || (round == 2 && idempotent) {
						session, _ := NewSession(recovery, "embedded")
						worker, _ := NewEmbeddedWorker(session, embedded.Worker{HeartbeatPeriod: time.Hour, Runner: embedded.RunnerFunc(func(ctx context.Context, a ports.Assignment) embedded.Outcome {
							assignments <- a
							if round == 0 {
								<-ctx.Done()
								close(cancelled)
								<-release
								return embedded.Outcome{Result: ports.OutcomeCancelled}
							}
							return embedded.Outcome{Result: ports.OutcomeSucceeded}
						})})
						stages = append(stages, []lane.Runner{session}, []lane.Runner{worker})
					}
					done := make(chan error, 1)
					go func() { done <- lifecycle.Run(process, stages...) }()
					joined := false
					defer func() {
						stop()
						unblock()
						if !joined {
							result(t, done)
						}
					}()
					// Runner entry precedes the Coordinator's durable Accept commit.
					var before domain.Execution
					var steps []domain.StepExecution
					for {
						before, steps, err = store.Get(ctx, "shutdown")
						want := domain.StepRunning
						if round == 2 && idempotent {
							want = domain.StepSucceeded
						}
						if lifecycle.Ready() && err == nil && len(steps) == 1 {
							settled := round == 1 || (round == 2 && !idempotent)
							if steps[0].State == want && (round != 2 || before.State == domain.ExecSucceeded) {
								settled = true
							}
							if settled {
								break
							}
						}
						if time.Now().After(deadline) {
							t.Fatal("lifecycle did not reach expected durable state")
						}
						time.Sleep(time.Millisecond)
					}
					if round == 0 || (round == 2 && idempotent) {
						select {
						case assignment := <-assignments:
							if assignment.AttemptID != steps[0].AttemptID || assignment.Lease.FencingToken != steps[0].FenceToken {
								t.Fatal("assignment differs from durable attempt")
							}
							if round == 0 {
								firstAssignment = assignment
							} else if assignment.AttemptID == firstAssignment.AttemptID || assignment.Lease.FencingToken <= firstAssignment.Lease.FencingToken || assignment.IdempotencyKey == "" || assignment.IdempotencyKey != firstAssignment.IdempotencyKey {
								t.Fatal("restart did not preserve idempotency key with a new fenced attempt")
							}
						case <-ctx.Done():
							t.Fatal("missing Worker assignment")
						}
					}
					events, err := store.Events(ctx, before.ID)
					if err != nil {
						t.Fatal(err)
					}
					if round == 0 {
						stoppedEvents = events
					}
					stop()
					if round == 0 {
						wait(t, cancelled)
						if lifecycle.Ready() {
							t.Fatal("admission remained open during shutdown")
						}
						select {
						case <-done:
							t.Fatal("lifecycle returned before the attempt joined")
						default:
						}
						if err := storage.db.PingContext(ctx); err != nil {
							t.Fatal("pool closed before the attempt joined")
						}
						unblock()
					}
					joined = true
					if err := result(t, done); err != nil {
						t.Fatal(err)
					}
					if storage.db.Stats().OpenConnections != 0 {
						t.Fatal("storage retained connections after shutdown")
					}
					after, afterSteps, err := store.Get(ctx, before.ID)
					afterEvents, eventsErr := store.Events(ctx, before.ID)
					if err != nil || eventsErr != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(steps, afterSteps) || !reflect.DeepEqual(events, afterEvents) {
						t.Fatal("process shutdown changed durable state or Events")
					}
				}()
			}
		})
	}
}
