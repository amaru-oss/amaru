package platform

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/rluders/lane/v2"
)

// Signal delivery must target an isolated process, never the shared test runner.
func TestLifecycleSignals(t *testing.T) {
	if mode := os.Getenv("AMARU_SIGNAL_CHILD"); mode != "" {
		phase, mode, ok := strings.Cut(mode, "/")
		if !ok {
			t.Fatal("missing signal phase")
		}
		shutdown := time.Second
		if mode == "timeout" {
			shutdown = 100 * time.Millisecond
		}
		l, err := NewLifecycle(time.Now().Add(3*time.Second), shutdown, nil)
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			if phase != "ready" {
				return
			}
			for !l.Ready() {
				time.Sleep(time.Millisecond)
			}
			_, _ = io.WriteString(os.Stdout, "signal\n")
		}()
		joined := make(chan struct{})
		var closedOnCancel, laterStarted atomic.Bool
		lateBudgetValid := make(chan bool, 1)
		cleanupDeadline := make(chan time.Time, 1)
		readinessJoined := make(chan struct{})
		order := make(chan int, 2)
		boom := errors.New("unregistered-signal-cleanup-fixture")
		err = l.Run(context.Background(), []lane.Runner{dependent{
			stop: func(ctx context.Context) error {
				order <- 1
				if mode == "timeout" {
					deadline, ok := ctx.Deadline()
					lateBudgetValid <- ok && deadline.Equal(<-cleanupDeadline) && errors.Is(ctx.Err(), context.DeadlineExceeded)
				}
				return nil
			},
		}}, []lane.Runner{dependent{
			start: func(ctx context.Context) error {
				<-ctx.Done()
				closedOnCancel.Store(!l.Ready())
				close(joined)
				return nil
			},
			ready: func(ctx context.Context) error {
				defer close(readinessJoined)
				if phase == "startup" {
					_, _ = io.WriteString(os.Stdout, "signal\n")
					<-ctx.Done()
					return ctx.Err()
				}
				return nil
			},
			stop: func(ctx context.Context) error {
				if l.Ready() || ctx.Err() != nil {
					return errors.New("invalid signal cleanup context or admission")
				}
				select {
				case <-joined:
				case <-ctx.Done():
					return ctx.Err()
				}
				select {
				case <-readinessJoined:
				case <-ctx.Done():
					return ctx.Err()
				}
				order <- 2
				switch mode {
				case "error":
					return boom
				case "panic":
					panic("unregistered-signal-panic-fixture")
				case "timeout":
					deadline, _ := ctx.Deadline()
					cleanupDeadline <- deadline
					select {}
				}
				return nil
			},
		}}, []lane.Runner{dependent{
			start: func(ctx context.Context) error {
				laterStarted.Store(true)
				<-ctx.Done()
				return nil
			},
		}})
		if laterStarted.Load() != (phase == "ready") {
			t.Fatal("dependent stage started before startup readiness")
		}
		var panicErr *lane.PanicError
		if (err == nil) != (mode == "clean") || (mode == "error" && !errors.Is(err, boom)) ||
			(mode == "panic" && !errors.As(err, &panicErr)) || l.Ready() || !closedOnCancel.Load() {
			t.Fatal("signal failure classification, admission or runner join changed")
		}
		if mode == "timeout" {
			select {
			case valid := <-lateBudgetValid:
				if !errors.Is(err, context.DeadlineExceeded) || !valid {
					t.Fatal("signal cleanup did not retain the exhausted aggregate budget")
				}
			case <-time.After(time.Second):
				t.Fatal("signal cleanup omitted the earlier stage after timeout")
			}
			if err := l.Run(context.Background()); err == nil {
				t.Fatal("signal cleanup timeout allowed in-process reuse")
			}
		}
		if len(order) != 2 || <-order != 2 || <-order != 1 {
			t.Fatal("signal cleanup did not run in reverse order after failure")
		}
		// The harness owns exit classification; production wiring remains pending.
		if err != nil {
			os.Exit(23)
		}
		os.Exit(0)
	}
	for _, signal := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		for _, scenario := range []string{"ready/clean", "ready/error", "ready/panic", "ready/timeout", "startup/clean", "startup/error", "startup/panic", "startup/timeout"} {
			_, mode, _ := strings.Cut(scenario, "/")
			t.Run(signal.String()+"/"+scenario, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLifecycleSignals$")
				cmd.Env = append(os.Environ(), "AMARU_SIGNAL_CHILD="+scenario)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				reader := bufio.NewReader(stdout)
				line, readErr := reader.ReadString('\n')
				if readErr != nil || line != "signal\n" || cmd.Process.Signal(signal) != nil {
					cancel()
					_ = cmd.Wait()
					t.Fatal("child did not reach signal checkpoint or receive signal")
				}
				out, readErr := io.ReadAll(reader)
				err = cmd.Wait()
				want := 0
				if mode != "clean" {
					want = 23
				}
				if ctx.Err() != nil || readErr != nil || cmd.ProcessState.ExitCode() != want ||
					(err == nil) != (want == 0) || len(out) != 0 || stderr.Len() != 0 {
					t.Fatal("signal child hung, misclassified exit or emitted unexpected output")
				}
			})
		}
	}
}
