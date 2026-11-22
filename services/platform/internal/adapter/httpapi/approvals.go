package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
)

// approvalPatterns are the served Approval operations. listApprovals is not served: it needs a
// Project-scoped Approval query that does not exist yet.
var approvalPatterns = []string{
	"GET /v1/projects/{projectId}/approvals/{approvalId}",
	"POST /v1/projects/{projectId}/approvals/{approvalId}/decision",
}

const maxDecisionBody = 8192

var digestRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var bareSHA256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// errUnrepresentable marks a stored Approval that the public contract cannot describe. It is
// answered as internal, never with a partial or invented body.
var errUnrepresentable = errors.New("approval is not representable in the public contract")

// ApprovalsHandler includes the compact Host/Origin/session/CSRF guard. access must use this
// Auth instance and the same authoritative Instance stores.
func ApprovalsHandler(auth *localauth.Auth, access *coordinator.ProjectAccess) (http.Handler, error) {
	if auth == nil || access == nil {
		return nil, domain.ErrInvalid
	}
	mux := http.NewServeMux()
	mux.HandleFunc(approvalPatterns[0], func(w http.ResponseWriter, r *http.Request) {
		project, id := domain.ProjectKey(r.PathValue("projectId")), r.PathValue("approvalId")
		approval, err := access.Approval(r.Context(), project, id)
		if err != nil {
			approvalError(w, err)
			return
		}
		writeApproval(w, r, access, project, approval)
	})
	mux.HandleFunc(approvalPatterns[1], func(w http.ResponseWriter, r *http.Request) {
		project, id := domain.ProjectKey(r.PathValue("projectId")), r.PathValue("approvalId")
		// The authorized read checks Project ownership before decoding any client body.
		if _, err := access.Approval(r.Context(), project, id); err != nil {
			approvalError(w, err)
			return
		}
		req, ok := decodeDecision(w, r)
		if !ok {
			return
		}
		req.ApprovalID = id
		// ProjectAccess rechecks live identity after body IO and sets the decider; the body
		// never names one.
		decided, _, err := access.DecideApproval(r.Context(), project, req)
		if err != nil {
			approvalError(w, err)
			return
		}
		writeApproval(w, r, access, project, decided.Approval)
	})
	return auth.Handler(mux), nil
}

func decodeDecision(w http.ResponseWriter, r *http.Request) (coordinator.DecideRequest, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		reject(w, 400, "invalid_request")
		return coordinator.DecideRequest{}, false
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDecisionBody))
	if err != nil {
		reject(w, 400, "invalid_request")
		return coordinator.DecideRequest{}, false
	}
	var body struct {
		Outcome        *string `json:"outcome"`
		EvidenceDigest *string `json:"evidenceDigest"`
		Rationale      *string `json:"rationale"`
		IdempotencyKey *string `json:"idempotencyKey"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || dec.More() || body.Outcome == nil || body.EvidenceDigest == nil || body.IdempotencyKey == nil {
		reject(w, 400, "invalid_request")
		return coordinator.DecideRequest{}, false
	}
	if _, err := dec.Token(); err != io.EOF {
		reject(w, 400, "invalid_request")
		return coordinator.DecideRequest{}, false
	}
	outcome := domain.ApprovalOutcome(*body.Outcome)
	key := *body.IdempotencyKey
	if (outcome != domain.OutcomeApproved && outcome != domain.OutcomeRejected) || !digestRe.MatchString(*body.EvidenceDigest) || key == "" || len(key) > 128 {
		reject(w, 400, "invalid_request")
		return coordinator.DecideRequest{}, false
	}
	req := coordinator.DecideRequest{Key: key, Outcome: outcome, EvidenceDigest: *body.EvidenceDigest}
	if body.Rationale != nil {
		req.Rationale = *body.Rationale
	}
	return req, true
}

// writeApproval maps a stored Approval to the public shape. A pinned definition item needs its
// Execution's committed revision, which is read through the same authorized boundary.
func writeApproval(w http.ResponseWriter, r *http.Request, access *coordinator.ProjectAccess, project domain.ProjectKey, a domain.Approval) {
	var pin *domain.DefinitionPin
	for _, it := range a.Evidence.Items {
		if it.Kind == "definition" {
			e, _, err := access.Execution(r.Context(), project, a.Gate.ExecutionID)
			if err != nil {
				approvalError(w, err)
				return
			}
			pin = e.Definition
			break
		}
	}
	out, err := wireApproval(a, pin)
	if err != nil {
		approvalError(w, err)
		return
	}
	respond(w, http.StatusOK, out)
}

type wirePrincipal struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Sponsor     string `json:"sponsor,omitempty"`
	ProjectRole string `json:"projectRole,omitempty"`
}

type wireDecision struct {
	Outcome   string        `json:"outcome"`
	Approver  wirePrincipal `json:"approver"`
	Rationale string        `json:"rationale,omitempty"`
	DecidedAt string        `json:"decidedAt"`
}

type wireApprovalBody struct {
	SchemaVersion int    `json:"schemaVersion"`
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	State         string `json:"state"`
	Gate          struct {
		Kind            string `json:"kind"`
		StepExecutionID string `json:"stepExecutionId"`
		Capability      string `json:"capability"`
	} `json:"gate"`
	Requester   wirePrincipal     `json:"requester"`
	Requirement map[string]string `json:"requirement"`
	Evidence    struct {
		Digest string           `json:"digest"`
		Items  []map[string]any `json:"items"`
	} `json:"evidence"`
	Decision    *wireDecision `json:"decision,omitempty"`
	RequestedAt string        `json:"requestedAt"`
}

func timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// wireApproval returns the Approval.json shape or errUnrepresentable. It never invents values:
// a voided Approval (the contract requires the voiding Event id, which is not recorded) and a
// definition without a committed revision are refused rather than approximated.
func wireApproval(a domain.Approval, pin *domain.DefinitionPin) (wireApprovalBody, error) {
	var out wireApprovalBody
	if a.State == domain.ApprovalVoided {
		return out, fmt.Errorf("%w: voided approval has no voidedBy event", errUnrepresentable)
	}
	if a.Gate.Kind != domain.GatePolicyRequired || a.Gate.StepID == "" || a.Gate.Capability == "" {
		return out, fmt.Errorf("%w: gate %q", errUnrepresentable, a.Gate.Kind)
	}
	if !digestRe.MatchString(a.Evidence.Digest) || len(a.Evidence.Items) == 0 {
		return out, fmt.Errorf("%w: evidence", errUnrepresentable)
	}
	out.SchemaVersion, out.ID, out.ProjectID, out.State = 1, a.ID, a.ProjectID, string(a.State)
	out.Gate.Kind, out.Gate.StepExecutionID, out.Gate.Capability = string(a.Gate.Kind), a.Gate.StepID, string(a.Gate.Capability)
	requester, err := principal(a.Requester)
	if err != nil {
		return out, err
	}
	out.Requester = requester
	out.Requirement = map[string]string{"minRole": string(a.MinRole)}
	out.Evidence.Digest = a.Evidence.Digest
	for _, it := range a.Evidence.Items {
		item, err := evidenceItem(it, pin)
		if err != nil {
			return out, err
		}
		out.Evidence.Items = append(out.Evidence.Items, item)
	}
	out.RequestedAt = timestamp(a.RequestedAt)
	if d := a.Decision; d != nil {
		if d.Approver == "" || string(d.Outcome) != string(a.State) {
			return out, fmt.Errorf("%w: decision", errUnrepresentable)
		}
		out.Decision = &wireDecision{Outcome: string(d.Outcome), Rationale: d.Rationale, DecidedAt: timestamp(d.DecidedAt),
			Approver: wirePrincipal{Kind: string(domain.PrincipalHuman), ID: d.Approver, ProjectRole: string(d.ApproverRole)}}
	} else if a.State != domain.ApprovalRequested && a.State != domain.ApprovalExpired {
		return out, fmt.Errorf("%w: %s without decision", errUnrepresentable, a.State)
	}
	return out, nil
}

func principal(p domain.Principal) (wirePrincipal, error) {
	out := wirePrincipal{Kind: string(p.Kind), ID: p.ID, Sponsor: p.Sponsor}
	switch p.Kind {
	case domain.PrincipalHuman:
		out.Sponsor, out.ProjectRole = "", string(p.Role)
	case domain.PrincipalAgent, domain.PrincipalTrigger:
		if p.Sponsor == "" {
			return out, fmt.Errorf("%w: %s requester without sponsor", errUnrepresentable, p.Kind)
		}
	case domain.PrincipalPlugin:
	default:
		return out, fmt.Errorf("%w: requester kind %q", errUnrepresentable, p.Kind)
	}
	if p.ID == "" {
		return out, fmt.Errorf("%w: requester id", errUnrepresentable)
	}
	return out, nil
}

// evidenceItem maps one recorded item. Agent hashes are recorded as bare SHA-256 hex and are
// rendered in the contract's Digest form; the value is unchanged.
func evidenceItem(it domain.EvidenceItem, pin *domain.DefinitionPin) (map[string]any, error) {
	hash := it.Hash
	if it.Kind == "agent" && bareSHA256Re.MatchString(hash) {
		hash = "sha256:" + hash
	}
	switch it.Kind {
	case "execution", "step_execution":
		if it.ID != "" && it.Hash == "" {
			return map[string]any{"kind": it.Kind, "id": it.ID}, nil
		}
	case "step_inputs":
		if it.ID == "" && digestRe.MatchString(hash) {
			return map[string]any{"kind": it.Kind, "hash": hash}, nil
		}
	case "skill", "agent", "context":
		if it.ID != "" && digestRe.MatchString(hash) {
			return map[string]any{"kind": it.Kind, "id": it.ID, "hash": hash}, nil
		}
	case "definition":
		// Only a committed pin that is exactly the recorded item can be described.
		if pin != nil && pin.WorkflowID == it.ID && pin.Hash == it.Hash && pin.Revision != "" && digestRe.MatchString(hash) {
			return map[string]any{"kind": it.Kind, "commit": pin.Revision, "contentHash": hash, "dirty": false}, nil
		}
	}
	return nil, fmt.Errorf("%w: evidence item %q", errUnrepresentable, it.Kind)
}

func approvalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotPermitted):
		reject(w, 403, "forbidden")
	case errors.Is(err, domain.ErrNotFound):
		reject(w, 404, "not_found")
	case errors.Is(err, domain.ErrEvidenceChanged), errors.Is(err, domain.ErrKeyReused),
		errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrConflict):
		reject(w, 409, "conflict")
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, domain.ErrReasonRequired):
		reject(w, 400, "invalid_request")
	default:
		reject(w, 500, "internal")
	}
}
