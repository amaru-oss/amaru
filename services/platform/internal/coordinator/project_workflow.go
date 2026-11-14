package coordinator

import (
	"context"
	"strings"

	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
)

// Workflow authorizes the Project before selecting its committed definition. Source is a
// trusted Instance dependency using the same attached Projects, never a request-supplied reader.
func (a *ProjectAccess) Workflow(ctx context.Context, project domain.ProjectKey, id, revision string, source ports.ProjectWorkflowSource) (domain.Workflow, error) {
	w, _, err := a.workflow(ctx, project, id, revision, source)
	return w.Workflow, err
}

func (a *ProjectAccess) workflow(ctx context.Context, project domain.ProjectKey, id, revision string, source ports.ProjectWorkflowSource) (ports.ProjectWorkflow, domain.Principal, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return ports.ProjectWorkflow{}, domain.Principal{}, err
	}
	if source == nil || !domain.ValidWorkflowID(id) || !domain.ValidDefinitionRevision(revision) {
		return ports.ProjectWorkflow{}, domain.Principal{}, domain.ErrInvalid
	}
	w, err := source.Workflow(ctx, project, id, revision)
	current, authErr := a.authorize(ctx, project)
	if authErr != nil {
		return ports.ProjectWorkflow{}, domain.Principal{}, authErr
	}
	if current.ID != by.ID {
		return ports.ProjectWorkflow{}, domain.Principal{}, domain.ErrNotPermitted
	}
	if err != nil {
		return ports.ProjectWorkflow{}, domain.Principal{}, err
	}
	if w.ID != id || w.Clone.ProjectKey != project || w.Clone.Root == "" || strings.ContainsRune(w.Clone.Root, 0) {
		return ports.ProjectWorkflow{}, domain.Principal{}, domain.ErrInvalid
	}
	if err := w.Validate(); err != nil {
		return ports.ProjectWorkflow{}, domain.Principal{}, domain.ErrInvalid
	}
	return w, by, nil
}

// RunWorkflowRequest names committed content; it carries no path, definition, Steps or actor.
// Revision is explicit so a retry cannot silently select a later HEAD with the same file name.
type RunWorkflowRequest struct {
	Key        string
	WorkflowID string
	Revision   string
	Variables  map[string]string
}

// RunWorkflow pins only the selected Project's definition and reuses Submit's Policy,
// attribution and atomic command outcome. A changed revision with an old key conflicts.
func (a *ProjectAccess) RunWorkflow(ctx context.Context, project domain.ProjectKey, req RunWorkflowRequest, source ports.ProjectWorkflowSource) (Submitted, bool, error) {
	w, by, err := a.workflow(ctx, project, req.WorkflowID, req.Revision, source)
	if err != nil {
		return Submitted{}, false, err
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return Submitted{}, false, err
	}
	if current.ID != by.ID {
		return Submitted{}, false, domain.ErrNotPermitted
	}
	// Keep the actor captured before loading through the final check and command attribution.
	return a.coordinator.Submit(ctx, SubmitRequest{ProjectID: string(project), By: by, Trigger: domain.TriggerManual, Key: req.Key, Workflow: &w.Workflow, Revision: req.Revision, CloneRoot: w.Clone.Root, Variables: req.Variables})
}
