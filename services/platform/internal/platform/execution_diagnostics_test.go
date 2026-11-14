package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/operational"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/coordinator/coordtest"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
	"github.com/rluders/amaru/services/platform/internal/worker/embedded"
	"github.com/rluders/lane/v2"
)

const executionSecret = "opaque-provider-output-fixture"

// Use a child so unexpected panic/error output cannot escape a buffer-only assertion.
func TestExecutionDiagnosticChannels(t *testing.T) {
	for _, mode := range []string{"success", "failure", "panic", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExecutionDiagnosticChild$")
			cmd.Env = append(os.Environ(), "AMARU_EXECUTION_DIAGNOSTIC_CHILD="+mode)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if bytes.Contains(stdout.Bytes(), []byte(executionSecret)) || bytes.Contains(stderr.Bytes(), []byte(executionSecret)) {
				t.Fatal("secret escaped process channels")
			}
			if err != nil {
				t.Fatal("diagnostic child failed; output withheld")
			}
			if stdout.Len() != 0 {
				t.Fatal("unexpected stdout")
			}
			lines := bytes.Split(bytes.TrimSpace(stderr.Bytes()), []byte{'\n'})
			if len(lines) != 2 {
				t.Fatal("expected exactly two Runner JSON records")
			}
			var attempt string
			for i, line := range lines {
				var record map[string]json.RawMessage
				if json.Unmarshal(line, &record) != nil || len(record) != 8 {
					t.Fatal("invalid diagnostic JSON fields")
				}
				field := func(key string) string {
					var v string
					if json.Unmarshal(record[key], &v) != nil {
						t.Fatal("missing string field", key)
					}
					return v
				}
				if _, err := time.Parse(time.RFC3339Nano, field("time")); err != nil {
					t.Fatal("invalid timestamp")
				}
				if field("instanceId") != "instance" || field("executionId") != "execution" || field("stepId") != "step" || field("projectId") != "project" || field("operation") != "worker.runner" {
					t.Fatal("incorrect trusted identity")
				}
				correlation := field("correlationId")
				if correlation == "" || (i > 0 && correlation != attempt) {
					t.Fatal("missing or inconsistent attempt correlation")
				}
				attempt = correlation
				want := "started"
				if i == 1 {
					want = map[string]string{"success": "succeeded", "failure": "failed", "panic": "panicked", "cancel": "cancelled"}[mode]
				}
				if field("outcome") != want {
					t.Fatal("incorrect Runner outcome")
				}
			}
		})
	}
}

func TestExecutionDiagnosticChild(t *testing.T) {
	mode := os.Getenv("AMARU_EXECUTION_DIAGNOSTIC_CHILD")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	// Exit directly after joining owners; testing's PASS line must not share stdout.
	if runExecutionDiagnosticChild(mode) != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func runExecutionDiagnosticChild(mode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store := fakes.NewExecutionStore()
	c := coordinator.New(store, time.Now, coordtest.Permissive)
	if _, err := c.Reconcile(ctx); err != nil {
		return err
	}
	recovery := &Recovery{coordinator: c, ready: make(chan struct{})}
	close(recovery.ready)
	execution := domain.Execution{ID: "execution", ProjectID: "project", State: domain.ExecQueued, CreatedAt: time.Now(), RequestedBy: coordtest.Requester, Trigger: domain.TriggerManual}
	if _, _, err := store.Create(ctx, execution, []domain.StepExecution{{ID: "step", ExecutionID: execution.ID, State: domain.StepQueued, Capability: "code.test", InputsJSON: []byte(`{"secret":"` + executionSecret + `"}`)}}); err != nil {
		return err
	}
	queue := make(chan operational.RunnerRecord, 2)
	entered := make(chan struct{})
	runner, err := operational.ObserveRunner("instance", embedded.RunnerFunc(func(ctx context.Context, a ports.Assignment) embedded.Outcome {
		close(entered)
		switch mode {
		case "panic":
			panic(executionSecret)
		case "failure":
			return embedded.Outcome{Result: ports.OutcomeFailed, ErrorCode: "unavailable", ErrorDetail: executionSecret}
		case "cancel":
			<-ctx.Done()
			return embedded.Outcome{Result: ports.OutcomeCancelled, ErrorDetail: executionSecret}
		}
		return embedded.Outcome{Result: ports.OutcomeSucceeded}
	}), queue)
	if err != nil {
		return err
	}
	session, err := NewSession(recovery, "embedded")
	if err != nil {
		return err
	}
	worker, err := NewEmbeddedWorker(session, embedded.Worker{Runner: runner})
	if err != nil {
		return err
	}
	lifecycle, err := NewLifecycle(time.Now().Add(3*time.Second), time.Second, nil)
	if err != nil {
		return err
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- lifecycle.Run(runCtx, []lane.Runner{session}, []lane.Runner{worker}) }()
	// Always join producers before returning or touching the diagnostic queue.
	joined := false
	defer func() {
		stop()
		if !joined {
			<-done
		}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	if mode == "cancel" {
		if _, _, err := c.Cancel(ctx, coordinator.CancelRequest{Key: "cancel-key", ExecutionID: execution.ID, By: domain.Actor{Kind: domain.ActorHuman}, ActorID: "human", Reason: executionSecret}); err != nil {
			return err
		}
	}
	wantExecution, wantStep := domain.ExecSucceeded, domain.StepSucceeded
	switch mode {
	case "failure", "panic":
		wantExecution, wantStep = domain.ExecFailed, domain.StepFailed
	case "cancel":
		wantExecution, wantStep = domain.ExecCancelled, domain.StepCancelled
	}
	for {
		e, steps, err := store.Get(ctx, execution.ID)
		if err != nil {
			return err
		}
		if e.State == wantExecution && steps[0].State == wantStep && lifecycle.Ready() {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
	stop()
	err = <-done
	joined = true
	if err != nil {
		return err
	}
	close(queue)
	logger, err := operational.New(os.Stderr) // Opaque payloads are unregistered: omission is required.
	if err != nil {
		return err
	}
	for record := range queue {
		e, steps, err := store.Get(ctx, record.Identity.ExecutionID)
		if err != nil {
			return err
		}
		if record.Identity.CorrelationID != steps[0].AttemptID {
			return errors.New("diagnostic attempt differs from committed Step")
		}
		record.Identity.ProjectID = e.ProjectID
		if err := logger.Write(operational.WithIdentity(ctx, record.Identity), record.Record); err != nil {
			return err
		}
	}
	return nil
}
