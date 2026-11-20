package operational

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestLifecycleConsumerDrainsWithTrustedIdentity(t *testing.T) {
	var sink bytes.Buffer
	logger, _ := New(&sink, "known-secret")
	queue := make(chan Record, 3)
	for _, outcome := range []string{"info", "warning", "error"} {
		queue <- Record{Operation: "process.lifecycle", Outcome: outcome, Cause: "known-secret"}
	}
	close(queue)
	ctx := WithIdentity(context.Background(), Identity{InstanceID: "instance", CorrelationID: "lifecycle"})
	if err := logger.ConsumeLifecycle(ctx, queue); err != nil {
		t.Fatal(err)
	}
	got := records(t, &sink)
	if len(got) != 3 {
		t.Fatal("closed queue was not drained")
	}
	for i, outcome := range []string{"info", "warning", "error"} {
		if got[i]["instanceId"] != "instance" || got[i]["correlationId"] != "lifecycle" || got[i]["outcome"] != outcome || got[i]["cause"] != "[REDACTED]" {
			t.Fatal("lost ordering, identity or redaction")
		}
	}
}

func TestLifecycleConsumerCancellationAndInvalidComposition(t *testing.T) {
	var sink bytes.Buffer
	logger, _ := New(&sink)
	trusted := WithIdentity(context.Background(), Identity{InstanceID: "instance", CorrelationID: "lifecycle"})
	queue := make(chan Record, 1)
	queue <- Record{Operation: "process.lifecycle", Outcome: "info"}
	for _, ctx := range []context.Context{context.Background(), WithIdentity(context.Background(), Identity{InstanceID: "instance"})} {
		if logger.ConsumeLifecycle(ctx, queue) == nil || len(queue) != 1 {
			t.Fatal("invalid identity consumed diagnostics")
		}
	}
	if logger.ConsumeLifecycle(trusted, nil) == nil || (*Logger)(nil).ConsumeLifecycle(trusted, queue) == nil {
		t.Fatal("invalid composition accepted")
	}
	ctx, cancel := context.WithCancel(trusted)
	cancel()
	if err := logger.ConsumeLifecycle(ctx, queue); !errors.Is(err, context.Canceled) || len(queue) != 1 || sink.Len() != 0 {
		t.Fatal("cancelled consumer drained pending records")
	}
	empty := make(chan Record)
	ctx, cancel = context.WithCancel(trusted)
	done := make(chan error, 1)
	go func() { done <- logger.ConsumeLifecycle(ctx, empty) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("idle consumer lost cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("idle consumer did not stop")
	}
}

func TestLifecycleConsumerCancellationWinsAgainstClosedQueue(t *testing.T) {
	var sink bytes.Buffer
	logger, _ := New(&sink)
	trusted := WithIdentity(context.Background(), Identity{InstanceID: "instance", CorrelationID: "lifecycle"})
	for range 20 {
		ctx := &cancelAfterInitialErr{Context: trusted, done: closedContext}
		queue := make(chan Record)
		close(queue)
		if err := logger.ConsumeLifecycle(ctx, queue); !errors.Is(err, context.Canceled) {
			t.Fatal("closed queue won over canceled context")
		}
	}
}

var closedContext = func() <-chan struct{} { ch := make(chan struct{}); close(ch); return ch }()

type cancelAfterInitialErr struct {
	context.Context
	done  <-chan struct{}
	calls atomic.Int32
}

func (c *cancelAfterInitialErr) Done() <-chan struct{} { return c.done }
func (c *cancelAfterInitialErr) Err() error {
	if c.calls.Add(1) == 1 {
		return nil
	}
	return context.Canceled
}

func TestLifecycleConsumerSinkFailureIsSafe(t *testing.T) {
	for _, writer := range []io.Writer{badWriter{}, badWriter{short: true}, panicWriter{}} {
		logger, _ := New(writer)
		queue := make(chan Record, 2)
		queue <- Record{Operation: "process.lifecycle", Outcome: "error"}
		queue <- Record{Operation: "process.lifecycle", Outcome: "info"}
		ctx := WithIdentity(context.Background(), Identity{InstanceID: "instance", CorrelationID: "lifecycle"})
		err := logger.ConsumeLifecycle(ctx, queue)
		if err == nil || err.Error() != "operational log write failed" || len(queue) != 1 {
			t.Fatal("unsafe sink failure or consumer continued after failure")
		}
	}
}

type stalledLifecycleWriter struct{ entered, release chan struct{} }

func (w stalledLifecycleWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}

func TestLifecycleConsumerStalledSinkDoesNotBlockProducers(t *testing.T) {
	writer := stalledLifecycleWriter{make(chan struct{}), make(chan struct{})}
	logger, _ := New(writer)
	queue := make(chan Record, 1)
	ctx, cancel := context.WithCancel(WithIdentity(context.Background(), Identity{InstanceID: "instance", CorrelationID: "lifecycle"}))
	done := make(chan error, 1)
	go func() { done <- logger.ConsumeLifecycle(ctx, queue) }()
	t.Cleanup(func() {
		cancel()
		close(writer.release)
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Error("released consumer lost cancellation")
		}
	})
	producer := slog.New(LifecycleHandler(queue))
	producer.Info("first")
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("consumer did not reach sink")
	}
	produced := make(chan struct{})
	go func() {
		defer close(produced)
		for range 100 {
			producer.Error("unregistered-secret")
		}
	}()
	select {
	case <-produced:
	case <-time.After(time.Second):
		t.Fatal("stalled sink blocked lifecycle producers")
	}
	if len(queue) != 1 {
		t.Fatal("bounded queue did not fill")
	}
}
