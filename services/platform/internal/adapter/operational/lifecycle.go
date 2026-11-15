package operational

import (
	"context"
	"errors"
	"log/slog"
)

// ConsumeLifecycle writes queued diagnostics with the trusted identity in ctx.
// Run it outside Lane callbacks. After producers join, close the queue and join
// this consumer to drain it; cancellation abandons queued records. A blocked
// writer cannot be interrupted here, so composition must bound its final join.
func (l *Logger) ConsumeLifecycle(ctx context.Context, queue <-chan Record) error {
	id := IdentityFrom(ctx)
	if l == nil || queue == nil || id.InstanceID == "" || id.CorrelationID == "" {
		return errors.New("lifecycle consumer requires logger, queue and identity")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case rec, ok := <-queue:
			if !ok {
				return nil
			}
			if err := l.Write(ctx, rec); err != nil {
				return err
			}
		}
	}
}

// LifecycleHandler drops messages, source locations and attributes without
// resolving their values. Only fixed severity labels reach the bounded queue.
// The composition owner drains it through ConsumeLifecycle with trusted identity;
// slow or failed sinks must never block Lane callbacks. Keep the queue open
// until all lifecycle producers have stopped. A nil/full queue drops records.
func LifecycleHandler(queue chan<- Record) slog.Handler {
	return lifecycleHandler{queue: queue}
}

type lifecycleHandler struct{ queue chan<- Record }

func (lifecycleHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h lifecycleHandler) Handle(_ context.Context, r slog.Record) error {
	outcome := "debug"
	switch {
	case r.Level >= slog.LevelError:
		outcome = "error"
	case r.Level >= slog.LevelWarn:
		outcome = "warning"
	case r.Level >= slog.LevelInfo:
		outcome = "info"
	}
	select {
	case h.queue <- Record{Operation: "process.lifecycle", Outcome: outcome}:
	default:
	}
	return nil
}

func (h lifecycleHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h lifecycleHandler) WithGroup(string) slog.Handler      { return h }
