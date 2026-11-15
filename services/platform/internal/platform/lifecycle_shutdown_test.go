package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/rluders/lane/v2"
)

// Stranded callbacks belong in a disposable process, never the test runner.
func TestLifecycleAggregateShutdownBudget(t *testing.T) {
	if mode := os.Getenv("AMARU_SHUTDOWN_BUDGET_CHILD"); mode != "" {
		l, err := NewLifecycle(time.Now().Add(3*time.Second), 100*time.Millisecond, nil)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		firstStop := make(chan context.Context, 1)
		type stopObservation struct {
			deadline time.Time
			err      error
		}
		lateStops := make(chan stopObservation, 2)
		late := dependent{stop: func(ctx context.Context) error {
			deadline, _ := ctx.Deadline()
			lateStops <- stopObservation{deadline: deadline, err: ctx.Err()}
			return ctx.Err()
		}}
		stuck := dependent{
			start: func(ctx context.Context) error {
				<-ctx.Done()
				if mode == "start" {
					select {}
				}
				return nil
			},
			stop: func(ctx context.Context) error {
				if l.Ready() || ctx.Err() != nil {
					t.Error("Stop must begin with admission closed and a fresh cleanup context")
				}
				firstStop <- ctx
				if mode == "stop" {
					select {}
				}
				<-ctx.Done()
				return ctx.Err()
			},
		}
		done := make(chan error, 1)
		go func() { done <- l.Run(ctx, []lane.Runner{late}, []lane.Runner{late}, []lane.Runner{stuck}) }()
		limit := time.After(3 * time.Second)
		for !l.Ready() {
			select {
			case <-done:
				t.Fatal("lifecycle exited before readiness")
			case <-limit:
				t.Fatal("lifecycle never became ready")
			case <-time.After(time.Millisecond):
			}
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) || l.Ready() {
				t.Fatal("exhausted shutdown did not fail closed")
			}
		case <-time.After(time.Second):
			t.Fatal("shutdown exceeded the aggregate budget tolerance")
		}
		cleanup := <-firstStop
		deadline, ok := cleanup.Deadline()
		if !ok {
			t.Fatal("cleanup has no deadline")
		}
		for range 2 {
			select {
			case late := <-lateStops:
				if !late.deadline.Equal(deadline) || !errors.Is(late.err, context.DeadlineExceeded) {
					t.Fatal("late Stop received a renewed cleanup budget")
				}
			case <-time.After(time.Second):
				t.Fatal("late Stop was omitted after budget exhaustion")
			}
		}
		if err := l.Run(context.Background()); err == nil {
			t.Fatal("failed lifecycle allowed in-process reuse")
		}
		if t.Failed() {
			return
		}
		// Test-owned exit classification; no production entrypoint exists yet.
		os.Exit(23)
	}
	for _, mode := range []string{"start", "stop"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLifecycleAggregateShutdownBudget$")
			cmd.Env = append(os.Environ(), "AMARU_SHUTDOWN_BUDGET_CHILD="+mode)
			out, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if ctx.Err() != nil || !errors.As(err, &exit) || exit.ExitCode() != 23 || len(out) != 0 {
				t.Fatal("child did not exit with the classified lifecycle failure, or emitted unexpected output")
			}
		})
	}
}
