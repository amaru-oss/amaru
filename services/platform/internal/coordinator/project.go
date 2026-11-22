package coordinator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
)

// ProjectAccess is the compact, single-Organization application boundary (ADR-0062).
// Authenticate must recheck the trusted transport identity on every call; request fields
// never establish authority. Internal scheduling continues to use Coordinator directly.
type ProjectAccess struct {
	coordinator  *Coordinator
	projects     ports.ProjectStore
	authenticate func(context.Context) (domain.Principal, error)
}

// NewProjectAccess binds public operations to the coordinator's authoritative store.
// Compact composition grants its authenticated local administrator all attached Projects;
// multi-Organization membership and non-owner access require a different authorization policy.
func NewProjectAccess(c *Coordinator, projects ports.ProjectStore, authenticate func(context.Context) (domain.Principal, error)) (*ProjectAccess, error) {
	if c == nil || projects == nil || authenticate == nil {
		return nil, fmt.Errorf("%w: Project access needs coordinator, Projects and authentication", domain.ErrInvalid)
	}
	return &ProjectAccess{c, projects, authenticate}, nil
}

func (a *ProjectAccess) administrator(ctx context.Context) (domain.Principal, error) {
	by, err := a.authenticate(ctx)
	if err != nil {
		return domain.Principal{}, err
	}
	if by.Kind != domain.PrincipalHuman || by.ID == "" || by.Role != domain.RoleOwner || by.Sponsor != "" {
		return domain.Principal{}, domain.ErrNotPermitted
	}
	if err := ctx.Err(); err != nil {
		return domain.Principal{}, err
	}
	return by, nil
}

func (a *ProjectAccess) authorize(ctx context.Context, project domain.ProjectKey) (domain.Principal, error) {
	by, err := a.administrator(ctx)
	if err != nil {
		return domain.Principal{}, err
	}
	if !project.Valid() {
		return domain.Principal{}, domain.ErrNotFound
	}
	p, err := a.projects.Get(ctx, project)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Principal{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Principal{}, err
	}
	if p.Key != project {
		return domain.Principal{}, domain.ErrNotFound
	}
	// Project lookup must not rebind the request to a replacement administrator.
	current, err := a.administrator(ctx)
	if err != nil {
		return domain.Principal{}, err
	}
	if current.ID != by.ID {
		return domain.Principal{}, domain.ErrNotPermitted
	}
	return by, nil
}

func (a *ProjectAccess) execution(ctx context.Context, project domain.ProjectKey, id string) (domain.Execution, []domain.StepExecution, error) {
	e, steps, err := a.coordinator.store.Get(ctx, id)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && (e.ID != id || e.ProjectID != string(project))) {
		return domain.Execution{}, nil, domain.ErrNotFound
	}
	if err != nil {
		return domain.Execution{}, nil, err
	}
	for _, step := range steps {
		if step.ExecutionID != id {
			return domain.Execution{}, nil, domain.ErrNotFound
		}
	}
	return e, steps, nil
}

// Execution refuses missing and foreign IDs identically, before returning child records.
func (a *ProjectAccess) Execution(ctx context.Context, project domain.ProjectKey, id string) (domain.Execution, []domain.StepExecution, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return domain.Execution{}, nil, err
	}
	e, steps, err := a.execution(ctx, project, id)
	if err != nil {
		return domain.Execution{}, nil, err
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return domain.Execution{}, nil, err
	}
	if current.ID != by.ID {
		return domain.Execution{}, nil, domain.ErrNotPermitted
	}
	return e, steps, nil
}

// ExecutionRecord is one Execution with its Steps in creation order.
type ExecutionRecord struct {
	Execution domain.Execution
	Steps     []domain.StepExecution
}

// Executions lists the selected Project's Executions, newest first. Identity is rechecked after
// the store reads; an Execution or Step from another Project fails the whole read rather than
// being filtered, because the store contract was broken.
func (a *ProjectAccess) Executions(ctx context.Context, project domain.ProjectKey) ([]ExecutionRecord, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return nil, err
	}
	ids, err := a.coordinator.store.ProjectExecutions(ctx, string(project))
	if err != nil {
		return nil, err
	}
	out := make([]ExecutionRecord, 0, len(ids))
	for _, id := range ids {
		e, steps, err := a.coordinator.store.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if e.ID != id || e.ProjectID != string(project) {
			return nil, errors.New("project executions: store returned a row outside the query")
		}
		for _, step := range steps {
			if step.ExecutionID != id {
				return nil, errors.New("project executions: store returned a step of another execution")
			}
		}
		out = append(out, ExecutionRecord{Execution: e, Steps: steps})
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return nil, err
	}
	if current.ID != by.ID {
		return nil, domain.ErrNotPermitted
	}
	return out, nil
}

// Submit derives attribution and Project identity from authorized context, not the body.
// Workflow/Steps remain trusted internal inputs; named committed Workflow runs use RunWorkflow.
// This method is not an HTTP create/run handler.
func (a *ProjectAccess) Submit(ctx context.Context, project domain.ProjectKey, req SubmitRequest) (Submitted, bool, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return Submitted{}, false, err
	}
	req.ProjectID, req.By = string(project), by
	req.Trigger = domain.TriggerManual
	return a.coordinator.Submit(ctx, req)
}

// Cancel checks ownership and refreshes authority before coordinator replay or mutation.
func (a *ProjectAccess) Cancel(ctx context.Context, project domain.ProjectKey, req CancelRequest) (Cancelled, bool, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return Cancelled{}, false, err
	}
	if _, _, err := a.execution(ctx, project, req.ExecutionID); err != nil {
		return Cancelled{}, false, err
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return Cancelled{}, false, err
	}
	if current.ID != by.ID {
		return Cancelled{}, false, domain.ErrNotPermitted
	}
	req.By, req.ActorID = domain.Actor{Kind: domain.ActorHuman}, by.ID
	return a.coordinator.Cancel(ctx, req)
}

func (a *ProjectAccess) approval(ctx context.Context, project domain.ProjectKey, id string) (domain.Approval, error) {
	approval, err := a.coordinator.store.Approval(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Approval{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Approval{}, err
	}
	if approval.ID != id || approval.ProjectID != string(project) {
		return domain.Approval{}, domain.ErrNotFound
	}
	if _, _, err := a.execution(ctx, project, approval.Gate.ExecutionID); err != nil {
		return domain.Approval{}, err
	}
	return approval, nil
}

func (a *ProjectAccess) Approval(ctx context.Context, project domain.ProjectKey, id string) (domain.Approval, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return domain.Approval{}, err
	}
	approval, err := a.approval(ctx, project, id)
	if err != nil {
		return domain.Approval{}, err
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return domain.Approval{}, err
	}
	if current.ID != by.ID {
		return domain.Approval{}, domain.ErrNotPermitted
	}
	return approval, nil
}

// Approvals lists the selected Project's Approvals in one state, newest request first. Identity
// is rechecked after the store read; a row from another Project or state fails the whole read
// rather than being filtered, because the store contract was broken.
func (a *ProjectAccess) Approvals(ctx context.Context, project domain.ProjectKey, state domain.ApprovalState) ([]domain.Approval, error) {
	switch state {
	case domain.ApprovalRequested, domain.ApprovalApproved, domain.ApprovalRejected, domain.ApprovalExpired, domain.ApprovalVoided:
	default:
		return nil, domain.ErrInvalid
	}
	by, err := a.authorize(ctx, project)
	if err != nil {
		return nil, err
	}
	list, err := a.coordinator.store.ProjectApprovals(ctx, string(project), state)
	if err != nil {
		return nil, err
	}
	for _, approval := range list {
		if approval.ProjectID != string(project) || approval.State != state {
			return nil, errors.New("project approvals: store returned a row outside the query")
		}
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return nil, err
	}
	if current.ID != by.ID {
		return nil, domain.ErrNotPermitted
	}
	return list, nil
}

func (a *ProjectAccess) DecideApproval(ctx context.Context, project domain.ProjectKey, req DecideRequest) (Decided, bool, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return Decided{}, false, err
	}
	if _, err := a.approval(ctx, project, req.ApprovalID); err != nil {
		return Decided{}, false, err
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return Decided{}, false, err
	}
	if current.ID != by.ID {
		return Decided{}, false, domain.ErrNotPermitted
	}
	req.By = by
	return a.coordinator.DecideApproval(ctx, req)
}

// Definition returns only the selected Project's pinned Execution definition.
func (a *ProjectAccess) Definition(ctx context.Context, project domain.ProjectKey, id string) (domain.Workflow, error) {
	e, _, err := a.Execution(ctx, project, id)
	if err != nil {
		return domain.Workflow{}, err
	}
	if e.Definition == nil {
		return domain.Workflow{}, domain.ErrNotFound
	}
	return e.Definition.Workflow()
}

// Evidence checks ownership before reading contexts, artifacts, Events or Approval audit.
// The reader is a trusted composition dependency, never supplied by a client.
func (a *ProjectAccess) Evidence(ctx context.Context, project domain.ProjectKey, id string, reader ports.EvidenceReader) (domain.ExecutionEvidence, error) {
	by, err := a.administrator(ctx)
	if err != nil {
		return domain.ExecutionEvidence{}, err
	}
	if _, _, err := a.Execution(ctx, project, id); err != nil {
		return domain.ExecutionEvidence{}, err
	}
	if reader == nil {
		return domain.ExecutionEvidence{}, domain.ErrInvalid
	}
	bundle, err := reader.Bundle(ctx, id)
	if err != nil {
		return domain.ExecutionEvidence{}, err
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return domain.ExecutionEvidence{}, err
	}
	if current.ID != by.ID {
		return domain.ExecutionEvidence{}, domain.ErrNotPermitted
	}
	if bundle.Execution.ID != id || bundle.Execution.ProjectID != string(project) {
		return domain.ExecutionEvidence{}, domain.ErrNotFound
	}
	return bundle, nil
}

// Events authorizes the Project and any Execution filter before exposing even a resync cursor.
// stream and window are trusted deployment dependencies; now comes from the coordinator.
func (a *ProjectAccess) Events(ctx context.Context, q domain.EventQuery, stream ports.EventStream, window time.Duration) (domain.EventPage, error) {
	by, err := a.authorize(ctx, domain.ProjectKey(q.ProjectID))
	if err != nil {
		return domain.EventPage{}, err
	}
	if q.ExecutionID != "" {
		if _, _, err := a.execution(ctx, domain.ProjectKey(q.ProjectID), q.ExecutionID); err != nil {
			return domain.EventPage{}, err
		}
	}
	if stream == nil {
		return domain.EventPage{}, domain.ErrInvalid
	}
	page, err := stream.ReadEvents(ctx, q, a.coordinator.now(), window)
	if err != nil {
		return domain.EventPage{}, err
	}
	current, err := a.authorize(ctx, domain.ProjectKey(q.ProjectID))
	if err != nil {
		return domain.EventPage{}, err
	}
	if current.ID != by.ID {
		return domain.EventPage{}, domain.ErrNotPermitted
	}
	for _, event := range page.Events {
		if event.ProjectID != q.ProjectID || (q.ExecutionID != "" && event.Event.ExecutionID != q.ExecutionID) {
			return domain.EventPage{}, domain.ErrNotFound
		}
	}
	return page, nil
}

// ResumeStep checks ownership and refreshes authority before replay or a human transition.
func (a *ProjectAccess) ResumeStep(ctx context.Context, project domain.ProjectKey, req HumanStepRequest) (domain.StepExecution, bool, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return domain.StepExecution{}, false, err
	}
	if _, _, err := a.execution(ctx, project, req.ExecutionID); err != nil {
		return domain.StepExecution{}, false, err
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return domain.StepExecution{}, false, err
	}
	if current.ID != by.ID {
		return domain.StepExecution{}, false, domain.ErrNotPermitted
	}
	req.By, req.ActorID = domain.Actor{Kind: domain.ActorHuman}, by.ID
	return a.coordinator.ResumeStep(ctx, req)
}

// AbandonStep checks ownership and refreshes authority before replay or a human transition.
func (a *ProjectAccess) AbandonStep(ctx context.Context, project domain.ProjectKey, req HumanStepRequest) (domain.StepExecution, bool, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return domain.StepExecution{}, false, err
	}
	if _, _, err := a.execution(ctx, project, req.ExecutionID); err != nil {
		return domain.StepExecution{}, false, err
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return domain.StepExecution{}, false, err
	}
	if current.ID != by.ID {
		return domain.StepExecution{}, false, domain.ErrNotPermitted
	}
	req.By, req.ActorID = domain.Actor{Kind: domain.ActorHuman}, by.ID
	return a.coordinator.AbandonStep(ctx, req)
}
