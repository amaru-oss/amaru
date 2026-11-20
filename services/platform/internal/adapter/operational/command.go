package operational

import (
	"context"
	"errors"

	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
)

// Commands decorates execution commands at composition time. Context identity must
// come from trusted command state. Requests, keys, reasons and raw errors are never
// logged. Logging failure cannot change or roll back a command's result.
type Commands struct {
	Next   *coordinator.Coordinator
	Logger *Logger
}

func (c Commands) Submit(ctx context.Context, req coordinator.SubmitRequest) (coordinator.Submitted, bool, error) {
	out, replay, err := c.Next.Submit(ctx, req)
	id := IdentityFrom(ctx)
	if err == nil {
		id.ProjectID = out.Execution.ProjectID
		id.ExecutionID = out.Execution.ID
	}
	c.record(WithIdentity(ctx, id), coordinator.CmdSubmit, err)
	return out, replay, err
}

func (c Commands) Cancel(ctx context.Context, req coordinator.CancelRequest) (coordinator.Cancelled, bool, error) {
	out, replay, err := c.Next.Cancel(ctx, req)
	c.record(ctx, coordinator.CmdCancel, err)
	return out, replay, err
}

func (c Commands) record(ctx context.Context, operation string, err error) {
	rec := Record{Operation: operation, Outcome: "succeeded"}
	if err != nil {
		rec.Outcome = "failed"
		rec.Cause = "command dependency failed"
		// Fixed causes retain diagnostic meaning without formatting arbitrary errors,
		// including URLs, authentication errors or hostile Error methods.
		for _, known := range []error{domain.ErrInvalid, domain.ErrInvalidTransition, domain.ErrNotPermitted, domain.ErrNotFound, domain.ErrKeyReused, domain.ErrConflict, context.Canceled, context.DeadlineExceeded} {
			if errors.Is(err, known) {
				rec.Cause = known.Error()
				rec.Outcome = "rejected"
				break
			}
		}
	}
	_ = c.Logger.Write(ctx, rec)
}
