package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
)

// executionPatterns are the served Execution operations: getExecution, cancelExecution and
// listExecutions.
var executionPatterns = []string{
	"GET /v1/projects/{projectId}/executions/{id}",
	"POST /v1/projects/{projectId}/executions/{id}/cancel",
	"GET /v1/projects/{projectId}/executions",
}

const (
	maxCancelBody   = 8192
	maxCancelReason = 4000
)

var capabilityIDRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// ExecutionsHandler includes the compact Host/Origin/session/CSRF guard. access must use this
// Auth instance and the same authoritative Instance stores.
func ExecutionsHandler(auth *localauth.Auth, access *coordinator.ProjectAccess) (http.Handler, error) {
	if auth == nil || access == nil {
		return nil, domain.ErrInvalid
	}
	mux := http.NewServeMux()
	mux.HandleFunc(executionPatterns[0], func(w http.ResponseWriter, r *http.Request) {
		project, id := domain.ProjectKey(r.PathValue("projectId")), r.PathValue("id")
		writeExecution(w, r, access, project, id)
	})
	mux.HandleFunc(executionPatterns[1], func(w http.ResponseWriter, r *http.Request) {
		project, id := domain.ProjectKey(r.PathValue("projectId")), r.PathValue("id")
		// The authorized read checks Project ownership before decoding any client body.
		if _, _, err := access.Execution(r.Context(), project, id); err != nil {
			accessError(w, err)
			return
		}
		req, ok := decodeCancel(w, r)
		if !ok {
			return
		}
		req.ExecutionID = id
		// ProjectAccess rechecks live identity after body IO and sets the actor; the body never
		// names one. A replay returns the recorded outcome without signalling again.
		if _, _, err := access.Cancel(r.Context(), project, req); err != nil {
			accessError(w, err)
			return
		}
		writeExecution(w, r, access, project, id)
	})
	mux.HandleFunc(executionPatterns[2], func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Query()) > 0 {
			reject(w, 400, "invalid_request")
			return
		}
		list, err := access.Executions(r.Context(), domain.ProjectKey(r.PathValue("projectId")))
		if err != nil {
			accessError(w, err)
			return
		}
		// One unrepresentable Execution fails the whole list; none is silently dropped.
		out := make([]wireExecutionBody, 0, len(list))
		for _, rec := range list {
			body, err := wireExecution(rec.Execution, rec.Steps)
			if err != nil {
				accessError(w, err)
				return
			}
			out = append(out, body)
		}
		respond(w, http.StatusOK, out)
	})
	return auth.Handler(mux), nil
}

func decodeCancel(w http.ResponseWriter, r *http.Request) (coordinator.CancelRequest, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		reject(w, 400, "invalid_request")
		return coordinator.CancelRequest{}, false
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCancelBody))
	if err != nil {
		reject(w, 400, "invalid_request")
		return coordinator.CancelRequest{}, false
	}
	var body struct {
		IdempotencyKey *string `json:"idempotencyKey"`
		Reason         *string `json:"reason"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || body.IdempotencyKey == nil {
		reject(w, 400, "invalid_request")
		return coordinator.CancelRequest{}, false
	}
	if _, err := dec.Token(); err != io.EOF {
		reject(w, 400, "invalid_request")
		return coordinator.CancelRequest{}, false
	}
	key := *body.IdempotencyKey
	if key == "" || utf8.RuneCountInString(key) > 128 {
		reject(w, 400, "invalid_request")
		return coordinator.CancelRequest{}, false
	}
	req := coordinator.CancelRequest{Key: key}
	if body.Reason != nil {
		if utf8.RuneCountInString(*body.Reason) > maxCancelReason {
			reject(w, 400, "invalid_request")
			return coordinator.CancelRequest{}, false
		}
		req.Reason = *body.Reason
	}
	return req, true
}

// writeExecution answers with the Execution's current state, read through the authorized
// boundary. After a cancel this is the state the command left, or a later one.
func writeExecution(w http.ResponseWriter, r *http.Request, access *coordinator.ProjectAccess, project domain.ProjectKey, id string) {
	e, steps, err := access.Execution(r.Context(), project, id)
	if err != nil {
		accessError(w, err)
		return
	}
	out, err := wireExecution(e, steps)
	if err != nil {
		accessError(w, err)
		return
	}
	respond(w, http.StatusOK, out)
}

type wireExecError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type wireStep struct {
	StepID     string         `json:"stepId"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Capability string         `json:"capability,omitempty"`
	Executable bool           `json:"executable"`
	State      string         `json:"state"`
	Reason     string         `json:"reason,omitempty"`
	StartedAt  string         `json:"startedAt,omitempty"`
	EndedAt    string         `json:"endedAt,omitempty"`
	Error      *wireExecError `json:"error,omitempty"`
}

type wireExecutionBody struct {
	ID         string         `json:"id"`
	ProjectID  string         `json:"projectId"`
	Capability string         `json:"capability,omitempty"`
	State      string         `json:"state"`
	CreatedAt  string         `json:"createdAt"`
	StartedAt  string         `json:"startedAt,omitempty"`
	EndedAt    string         `json:"endedAt,omitempty"`
	Error      *wireExecError `json:"error,omitempty"`
	Steps      []wireStep     `json:"steps"`
}

// wireExecution returns the Execution.json shape or errUnrepresentable. It never invents values:
// `steps` is authoritative; `capability` is set only for a single-Step Execution; `providerId`,
// `input`, `outputs`, `checkout` and `workItemId` are not recorded on the domain Execution and
// are omitted. `workflowId` is omitted because domain Workflow ids are slugs and the contract's
// WorkflowId is `WF-<digits>`. A Step with an Agent snapshot is refused: no product path binds
// one yet, so its mapping lands with agent Steps.
func wireExecution(e domain.Execution, steps []domain.StepExecution) (wireExecutionBody, error) {
	var out wireExecutionBody
	if e.ID == "" || e.ProjectID == "" || !e.State.Valid() || e.CreatedAt.IsZero() {
		return out, fmt.Errorf("%w: execution identity or state", errUnrepresentable)
	}
	if e.State.Terminal() != (e.EndedAt != nil) || (e.State == domain.ExecQueued && e.StartedAt != nil) {
		return out, fmt.Errorf("%w: execution timestamps for %s", errUnrepresentable, e.State)
	}
	out.ID, out.ProjectID, out.State, out.CreatedAt = e.ID, e.ProjectID, string(e.State), timestamp(e.CreatedAt)
	out.StartedAt, out.EndedAt = optionalTimestamp(e.StartedAt), optionalTimestamp(e.EndedAt)
	execErr, err := wireError(e.Error, e.State == domain.ExecFailed)
	if err != nil {
		return out, err
	}
	out.Error = execErr
	var defined map[string]domain.WorkflowStep
	if e.Definition != nil {
		var w domain.Workflow
		if err := json.Unmarshal(e.Definition.Snapshot, &w); err != nil {
			return out, fmt.Errorf("%w: pinned definition: %v", errUnrepresentable, err)
		}
		defined = make(map[string]domain.WorkflowStep, len(w.Steps))
		for _, s := range w.Steps {
			defined[s.ID] = s
		}
	}
	out.Steps = make([]wireStep, 0, len(steps))
	for _, st := range steps {
		ws, err := wireStepExecution(e, st, defined)
		if err != nil {
			return out, err
		}
		out.Steps = append(out.Steps, ws)
	}
	if len(out.Steps) == 1 {
		out.Capability = out.Steps[0].Capability
	}
	return out, nil
}

// wireStepExecution maps one Step. Its stepId is the Workflow step id (the domain id is
// "<execution id>.<step id>"); name and type come from the pinned definition. A Step submitted
// without a Workflow names only a Capability, so it is described as a tool step named by its id.
// Skipped and refused Steps are not executable; every other Step was planned to run.
func wireStepExecution(e domain.Execution, st domain.StepExecution, defined map[string]domain.WorkflowStep) (wireStep, error) {
	var out wireStep
	stepID, ok := strings.CutPrefix(st.ID, e.ID+".")
	if !ok || stepID == "" || st.ExecutionID != e.ID || !st.State.Valid() {
		return out, fmt.Errorf("%w: step identity or state", errUnrepresentable)
	}
	if st.Agent != nil {
		return out, fmt.Errorf("%w: agent snapshot", errUnrepresentable)
	}
	out.StepID, out.Name, out.Type = stepID, stepID, string(domain.StepTypeTool)
	if defined != nil {
		ds, ok := defined[stepID]
		if !ok || ds.Name == "" {
			return out, fmt.Errorf("%w: step %s is not in the pinned definition", errUnrepresentable, stepID)
		}
		out.Name, out.Type = ds.Name, string(ds.Type)
	}
	if st.Capability != "" && !capabilityIDRe.MatchString(st.Capability) {
		return out, fmt.Errorf("%w: step capability", errUnrepresentable)
	}
	out.Capability, out.State, out.Reason = st.Capability, string(st.State), st.Reason
	out.Executable = st.State != domain.StepSkipped && st.State != domain.StepRefused
	if (st.State == domain.StepSkipped || st.State == domain.StepRefused) && st.Reason == "" {
		return out, fmt.Errorf("%w: %s step without reason", errUnrepresentable, st.State)
	}
	switch st.State {
	case domain.StepSucceeded, domain.StepFailed, domain.StepTimedOut, domain.StepCancelled:
		if st.EndedAt == nil {
			return out, fmt.Errorf("%w: %s step without end", errUnrepresentable, st.State)
		}
	}
	out.StartedAt, out.EndedAt = optionalTimestamp(st.StartedAt), optionalTimestamp(st.EndedAt)
	stepErr, err := wireError(st.Error, st.State == domain.StepFailed)
	if err != nil {
		return out, err
	}
	out.Error = stepErr
	return out, nil
}

func wireError(err *domain.ExecError, required bool) (*wireExecError, error) {
	if err == nil {
		if required {
			return nil, fmt.Errorf("%w: failure without error", errUnrepresentable)
		}
		return nil, nil
	}
	if err.Code == "" || err.Message == "" {
		return nil, fmt.Errorf("%w: incomplete error", errUnrepresentable)
	}
	return &wireExecError{Code: err.Code, Message: err.Message}, nil
}

func optionalTimestamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return timestamp(*t)
}
