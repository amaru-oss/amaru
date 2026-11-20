package domain

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// EvidenceGap names missing Evidence, including kinds with no recorder yet.
// A gap is listed so a reader does not take absence for "there was nothing".
type EvidenceGap struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

// Kinds of missing Evidence named by FEAT-ART-002.
const (
	GapContext   = "context"
	GapDiff      = "diff"
	GapLog       = "log"
	GapCheck     = "check"
	GapUsage     = "usage"
	GapWorkspace = "workspace_snapshot"
)

// evidenceGaps is the list of kinds with no recorder. It is a function so a caller cannot edit it.
func evidenceGaps() []EvidenceGap {
	const no = "the Platform records no %s yet"
	return []EvidenceGap{
		{GapCheck, fmt.Sprintf(no, "check results")},
		{GapDiff, fmt.Sprintf(no, "diffs")},
		{GapLog, fmt.Sprintf(no, "Step logs")},
		{GapWorkspace, fmt.Sprintf(no, "workspace snapshots")},
	}
}

// StepEvidence is what the Platform holds about one Step of an Execution. The Step's inputs are
// not copied, because they may hold values the Evidence reader should not see; InputsHash lets a
// reader check a copy of them.
type StepEvidence struct {
	Usage      []ResultCapture `json:"usage,omitempty"`
	UsageGap   string          `json:"usageGap,omitempty"`
	ID         string          `json:"id"`
	State      StepState       `json:"state"`
	Reason     string          `json:"reason,omitempty"`
	Capability string          `json:"capability,omitempty"`
	AttemptID  string          `json:"attemptId,omitempty"`
	StartedAt  *time.Time      `json:"startedAt,omitempty"`
	EndedAt    *time.Time      `json:"endedAt,omitempty"`
	Error      *ExecError      `json:"error,omitempty"`
	InputsHash string          `json:"inputsHash,omitempty"`
	// Skills are the Skills the Step pinned; Agent is the Agent definition it ran as, resolved
	// when the Step was planned.
	Skills []SkillRef     `json:"skills,omitempty"`
	Agent  *ResolvedAgent `json:"agent,omitempty"`
	// Context is what the Agent was told; nil when the Platform assembled none for the Step.
	Context    *StepContext `json:"context,omitempty"`
	ContextGap string       `json:"contextGap,omitempty"`
	// Artifacts are the versions the Step produced, ordered by Artifact id.
	Artifacts []ArtifactVersion `json:"artifacts,omitempty"`
}

// ExecutionEvidence is the bundle of everything the Platform holds about one Execution, so one
// query answers an audit (FEAT-ART-002). It has no clock of its own: the same stored data always
// gives the same bundle and the same Digest.
type ExecutionEvidence struct {
	Execution Execution        `json:"execution"`
	Steps     []StepEvidence   `json:"steps"`
	Events    []ExecutionEvent `json:"events"`
	// Approvals are the Approvals that gate the Execution's Steps, each with the Evidence its
	// approver was shown and the decision; Audit is their entries in the audit log.
	Approvals []Approval   `json:"approvals"`
	Audit     []AuditEvent `json:"audit"`
	// Final is true once the Execution is in a terminal state. A bundle read earlier is a view of
	// a run still in progress.
	Final bool `json:"final"`
	// Gaps lists missing Evidence kinds, including kinds with no recorder yet.
	Gaps []EvidenceGap `json:"gaps"`
	// Digest is "sha256:" and the hash of the bundle's JSON with Digest empty.
	Digest string `json:"digest"`
}

// ExecutionEvidenceInput is what the Platform read from its stores. Build checks that it is about
// one Execution.
type ExecutionEvidenceInput struct {
	Execution Execution
	Steps     []StepExecution
	Events    []ExecutionEvent
	Approvals []Approval
	// Audit holds the audit entries of the Approvals only.
	Audit     []AuditEvent
	Contexts  []StepContext
	Artifacts []ArtifactVersion
	Usage     []ResultCapture
}

// BuildExecutionEvidence assembles the bundle. It returns ErrInvalid when a part belongs to
// another Execution or Step, Step/context identities are ambiguous, or a pinned definition or
// recorded context is damaged. Steps keep their creation order, Approvals their request order and
// Events their strictly increasing Seq order; Artifacts are grouped under their Step. The returned bundle owns its
// contents; later input edits cannot invalidate its Digest.
func BuildExecutionEvidence(in ExecutionEvidenceInput) (ExecutionEvidence, error) {
	id := in.Execution.ID
	if id == "" {
		return ExecutionEvidence{}, invalid("evidence: the execution has no id")
	}
	if in.Execution.Definition != nil {
		if _, err := in.Execution.Definition.Workflow(); err != nil {
			return ExecutionEvidence{}, fmt.Errorf("evidence: pinned definition: %w", err)
		}
	}
	steps := make(map[string]int, len(in.Steps))
	out := ExecutionEvidence{
		Execution: in.Execution,
		Steps:     make([]StepEvidence, len(in.Steps)),
		Events:    slices.Clone(in.Events),
		Approvals: slices.Clone(in.Approvals),
		Audit:     slices.Clone(in.Audit),
		Final:     in.Execution.State.Terminal(),
		Gaps:      evidenceGaps(),
	}
	for i, st := range in.Steps {
		if _, exists := steps[st.ID]; st.ID == "" || exists {
			return ExecutionEvidence{}, invalid("evidence: empty or duplicate step id %q", st.ID)
		}
		if st.ExecutionID != id {
			return ExecutionEvidence{}, invalid("evidence: step %s belongs to execution %s, not %s", st.ID, st.ExecutionID, id)
		}
		steps[st.ID] = i
		se := StepEvidence{
			ID: st.ID, State: st.State, Reason: st.Reason, Capability: st.Capability, AttemptID: st.AttemptID,
			StartedAt: st.StartedAt, EndedAt: st.EndedAt, Error: st.Error,
			Skills: slices.Clone(st.Skills), Agent: st.Agent,
		}
		if len(st.InputsJSON) > 0 {
			sum := sha256.Sum256(st.InputsJSON)
			se.InputsHash = "sha256:" + hex.EncodeToString(sum[:])
		}
		out.Steps[i] = se
	}
	seenUsage := map[string]bool{}
	for _, c := range in.Usage {
		i, ok := steps[c.StepID]
		if !ok || seenUsage[c.AttemptID] {
			return ExecutionEvidence{}, invalid("evidence: usage references unknown step")
		}
		seenUsage[c.AttemptID] = true
		// Historical attempts remain attributable to the Step; do not bind to latest.
		if c.ExecutionID != id || c.ProjectID != in.Execution.ProjectID || c.EnvironmentID != in.Execution.EnvironmentID || c.AttemptID == "" || c.CapturedAt.IsZero() {
			return ExecutionEvidence{}, invalid("evidence: usage scope mismatch")
		}
		if err := ValidateUsage(c.Reports); err != nil {
			return ExecutionEvidence{}, err
		}
		out.Steps[i].Usage = append(out.Steps[i].Usage, c)
	}
	missingUsage := false
	for i := range out.Steps {
		// Historical receipts remain visible, but cannot cover the current attempt.
		if len(out.Steps[i].Usage) == 0 || in.Steps[i].AttemptID != "" && !hasUsageAttempt(out.Steps[i].Usage, in.Steps[i].AttemptID) {
			out.Steps[i].UsageGap = "no accepted Worker result captured; usage unknown"
			missingUsage = true
		} else {
			for _, c := range out.Steps[i].Usage {
				if len(c.Reports) == 0 {
					out.Steps[i].UsageGap = "accepted Worker result contains no usage reports; usage unknown"
					missingUsage = true
				}
			}
		}
	}
	if missingUsage {
		out.Gaps = append(out.Gaps, EvidenceGap{GapUsage, "one or more Steps have unknown usage; see usageGap"})
	}
	for _, c := range in.Contexts {
		i, ok := steps[c.StepID]
		if !ok || c.ExecutionID != id {
			return ExecutionEvidence{}, invalid("evidence: context of step %s/%s is not a step of execution %s", c.ExecutionID, c.StepID, id)
		}
		if out.Steps[i].Context != nil {
			return ExecutionEvidence{}, invalid("evidence: duplicate context for step %s", c.StepID)
		}
		if err := c.Validate(); err != nil {
			return ExecutionEvidence{}, fmt.Errorf("evidence: recorded context: %w", err)
		}
		c := c
		out.Steps[i].Context = &c
	}
	missingContext := false
	for i := range out.Steps {
		if out.Steps[i].Context == nil {
			out.Steps[i].ContextGap = "no context recorded for this Step"
			missingContext = true
		}
	}
	if missingContext {
		out.Gaps = append(out.Gaps, EvidenceGap{GapContext, "one or more Steps have no recorded context; see contextGap"})
	}
	type artifactKey struct {
		id      string
		version int64
	}
	artifacts := make(map[artifactKey]bool, len(in.Artifacts))
	for _, v := range in.Artifacts {
		i, ok := steps[v.StepID]
		if !ok || v.ExecutionID != id {
			return ExecutionEvidence{}, invalid("evidence: artifact %s v%d of step %s/%s is not a step of execution %s", v.ArtifactID, v.Version, v.ExecutionID, v.StepID, id)
		}
		if v.Version < 1 {
			return ExecutionEvidence{}, invalid("evidence: artifact version must be positive")
		}
		if err := v.Validate(); err != nil {
			return ExecutionEvidence{}, fmt.Errorf("evidence: recorded artifact: %w", err)
		}
		key := artifactKey{v.ArtifactID, v.Version}
		if artifacts[key] {
			return ExecutionEvidence{}, invalid("evidence: duplicate artifact %s v%d", v.ArtifactID, v.Version)
		}
		artifacts[key] = true
		out.Steps[i].Artifacts = append(out.Steps[i].Artifacts, v)
	}
	for i := range out.Steps {
		slices.SortStableFunc(out.Steps[i].Artifacts, func(a, b ArtifactVersion) int {
			if a.ArtifactID != b.ArtifactID {
				if a.ArtifactID < b.ArtifactID {
					return -1
				}
				return 1
			}
			return cmp.Compare(a.Version, b.Version)
		})
	}
	approvals := make(map[string]bool, len(in.Approvals))
	for _, a := range in.Approvals {
		if a.ID == "" || approvals[a.ID] {
			return ExecutionEvidence{}, invalid("evidence: empty or duplicate approval id %q", a.ID)
		}
		if a.ProjectID != in.Execution.ProjectID {
			return ExecutionEvidence{}, invalid("evidence: approval project does not match execution")
		}
		if a.Gate.ExecutionID != id {
			return ExecutionEvidence{}, invalid("evidence: approval %s gates execution %s, not %s", a.ID, a.Gate.ExecutionID, id)
		}
		if _, ok := steps[a.Gate.StepID]; !ok {
			return ExecutionEvidence{}, invalid("evidence: approval %s gates unknown step %s", a.ID, a.Gate.StepID)
		}
		approvals[a.ID] = true
	}
	for _, ev := range in.Audit {
		if ev.ProjectID != in.Execution.ProjectID {
			return ExecutionEvidence{}, invalid("evidence: approval audit project does not match execution")
		}
		if ev.TargetType != "approval" || !approvals[ev.TargetID] {
			return ExecutionEvidence{}, invalid("evidence: audit entry %d (%s %s) is not about an approval of execution %s", ev.Seq, ev.TargetType, ev.TargetID, id)
		}
	}
	var lastEventSeq int64
	for _, ev := range in.Events {
		if ev.Seq <= lastEventSeq {
			return ExecutionEvidence{}, invalid("evidence: event sequence %d does not advance past %d", ev.Seq, lastEventSeq)
		}
		lastEventSeq = ev.Seq
		if ev.ExecutionID != id {
			return ExecutionEvidence{}, invalid("evidence: event %d belongs to execution %s, not %s", ev.Seq, ev.ExecutionID, id)
		}
		if ev.StepID != "" {
			if _, ok := steps[ev.StepID]; !ok {
				return ExecutionEvidence{}, invalid("evidence: event %d references unknown step %s", ev.Seq, ev.StepID)
			}
		}
	}
	if out.Events == nil {
		out.Events = []ExecutionEvent{}
	}
	if out.Approvals == nil {
		out.Approvals = []Approval{}
	}
	if out.Audit == nil {
		out.Audit = []AuditEvent{}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return ExecutionEvidence{}, fmt.Errorf("evidence: encode: %w", err)
	}
	// Return the exact hashed representation without pointers into the source records.
	var snapshot ExecutionEvidence
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return ExecutionEvidence{}, fmt.Errorf("evidence: snapshot: %w", err)
	}
	sum := sha256.Sum256(raw)
	snapshot.Digest = "sha256:" + hex.EncodeToString(sum[:])
	return snapshot, nil
}

// hasUsageAttempt is scoped to the Step: another Step's receipt is not coverage.
func hasUsageAttempt(captures []ResultCapture, attempt string) bool {
	return slices.ContainsFunc(captures, func(c ResultCapture) bool { return c.AttemptID == attempt })
}
