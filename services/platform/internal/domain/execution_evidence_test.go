package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestExecutionEvidenceOwnsItsHashedContents(t *testing.T) {
	for _, direction := range []string{"input", "bundle"} {
		t.Run(direction, func(t *testing.T) {
			in := evidenceInput(t)
			at := in.Execution.CreatedAt
			in.Execution.Definition = &DefinitionPin{Snapshot: json.RawMessage(`{"id":"workflow"}`)}
			in.Execution.StartedAt = &at
			in.Steps[0].Agent = &ResolvedAgent{ProviderPreference: []string{"first"}, WorktreeCommands: []string{"go test"}, Skills: []SkillUse{{Name: "skill"}}}
			in.Steps[0].Error = &ExecError{Code: "original"}
			in.Contexts[0].Items = []ContextItem{{Name: "original"}}
			in.Approvals[0].Decision = &ApprovalDecision{Rationale: "original"}
			zero := uint64(0)
			credits := 1.5
			in.Usage = []ResultCapture{{ExecutionID: "e1", StepID: "e1.a", AttemptID: "a1", CapturedAt: at, Reports: []UsageReport{{
				Provider: "original", InputTokens: &IntMeasurement{Source: UsageReported, Value: &zero},
				Credits: &DoubleMeasurement{Source: UsageReported, Value: &credits},
				Cost:    &Cost{Source: UsageReported, Amount: "1.00", Currency: "USD"},
			}}}}
			bundle, err := BuildExecutionEvidence(in)
			if err != nil {
				t.Fatal(err)
			}
			encode := func(v any) []byte {
				t.Helper()
				raw, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}
				return raw
			}
			beforeInput, beforeBundle := encode(in), encode(bundle)
			unhashed := bundle
			unhashed.Digest = ""
			if ContentHash(encode(unhashed)) != bundle.Digest {
				t.Fatal("returned representation does not match its digest")
			}
			e, agent, execErr, ctx, approval, usage := &in.Execution, in.Steps[0].Agent, in.Steps[0].Error, &in.Contexts[0], &in.Approvals[0], &in.Usage[0]
			if direction == "bundle" {
				e, agent, execErr, ctx, approval, usage = &bundle.Execution, bundle.Steps[0].Agent, bundle.Steps[0].Error, bundle.Steps[0].Context, &bundle.Approvals[0], &bundle.Steps[0].Usage[0]
			}
			e.Definition.Snapshot[2] = 'x'
			*e.StartedAt = at.Add(time.Hour)
			agent.ProviderPreference[0], agent.WorktreeCommands[0], agent.Skills[0].Name = "changed", "changed", "changed"
			execErr.Code, ctx.Items[0].Name = "changed", "changed"
			approval.Evidence.Items[0].ID, approval.Decision.Rationale = "changed", "changed"
			usage.Reports[0].Provider = "changed"
			*usage.Reports[0].InputTokens.Value = 99
			*usage.Reports[0].Credits.Value = 99
			usage.Reports[0].Cost.Amount = "99"
			if direction == "input" && !bytes.Equal(beforeBundle, encode(bundle)) {
				t.Fatal("input mutation changed the already hashed bundle")
			}
			if direction == "bundle" && !bytes.Equal(beforeInput, encode(in)) {
				t.Fatal("bundle mutation changed the source records")
			}
		})
	}
}

func evidenceInput(t *testing.T) ExecutionEvidenceInput {
	t.Helper()
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	ev, err := NewEvidence(EvidenceItem{Kind: "execution", ID: "e1"})
	if err != nil {
		t.Fatal(err)
	}
	ap, err := NewApproval(at, "e1.a", "p1", ApprovalGate{Kind: GatePolicyRequired, ExecutionID: "e1", StepID: "e1.b", Capability: "git.push"},
		Principal{Kind: PrincipalHuman, ID: "u1", Role: RoleContributor}, ev)
	if err != nil {
		t.Fatal(err)
	}
	return ExecutionEvidenceInput{
		Execution: Execution{ID: "e1", State: ExecSucceeded, CreatedAt: at},
		Steps: []StepExecution{
			{ID: "e1.a", ExecutionID: "e1", State: StepSucceeded, InputsJSON: []byte(`{"token":"s3cret"}`)},
			{ID: "e1.b", ExecutionID: "e1", State: StepSucceeded},
		},
		Events:    []ExecutionEvent{{ExecutionID: "e1", Seq: 1, Kind: EventStepLeased, StepID: "e1.a", At: at}},
		Approvals: []Approval{ap},
		Audit:     []AuditEvent{{Seq: 3, At: at, Kind: AuditApprovalRequested, Actor: "u1", ActorKind: ActorHuman, TargetType: "approval", TargetID: "e1.a"}},
		Contexts:  []StepContext{{ExecutionID: "e1", StepID: "e1.a", Format: "amaru-context/1", Hash: "h", Document: "doc", CreatedAt: at}},
		Artifacts: []ArtifactVersion{
			{ArtifactID: "z", Version: 1, ExecutionID: "e1", StepID: "e1.a"},
			{ArtifactID: "a", Version: 2, ExecutionID: "e1", StepID: "e1.a"},
			{ArtifactID: "a", Version: 1, ExecutionID: "e1", StepID: "e1.a"},
			{ArtifactID: "m", Version: 1, ExecutionID: "e1", StepID: "e1.b"},
		},
	}
}

func TestBuildExecutionEvidenceGroupsEverythingUnderItsStep(t *testing.T) {
	b, err := BuildExecutionEvidence(evidenceInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Steps) != 2 || b.Steps[0].ID != "e1.a" || b.Steps[1].ID != "e1.b" {
		t.Fatalf("steps keep creation order: %+v", b.Steps)
	}
	a := b.Steps[0]
	if a.Context == nil || a.Context.Document != "doc" || b.Steps[1].Context != nil {
		t.Errorf("context belongs to e1.a only: %+v / %+v", a.Context, b.Steps[1].Context)
	}
	var got []string
	for _, v := range a.Artifacts {
		got = append(got, v.ArtifactID+string(rune('0'+v.Version)))
	}
	if len(got) != 3 || got[0] != "a1" || got[1] != "a2" || got[2] != "z1" || len(b.Steps[1].Artifacts) != 1 {
		t.Errorf("artifacts ordered by id then version, under their own step: %v", got)
	}
	if !b.Final || len(b.Events) != 1 || len(b.Approvals) != 1 || len(b.Audit) != 1 {
		t.Errorf("final %v events %d approvals %d audit %d", b.Final, len(b.Events), len(b.Approvals), len(b.Audit))
	}
}

func TestBuildExecutionEvidenceDoesNotCopyStepInputs(t *testing.T) {
	b, err := BuildExecutionEvidence(evidenceInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if b.Steps[0].InputsHash != ContentHash([]byte(`{"token":"s3cret"}`)) || b.Steps[1].InputsHash != "" {
		t.Errorf("inputs hash %q / %q", b.Steps[0].InputsHash, b.Steps[1].InputsHash)
	}
}

func TestBuildExecutionEvidenceListsWhatItCannotHold(t *testing.T) {
	b, _ := BuildExecutionEvidence(evidenceInput(t))
	want := map[string]bool{GapCheck: true, GapDiff: true, GapLog: true, GapUsage: true, GapWorkspace: true, GapContext: true}
	for _, g := range b.Gaps {
		if !want[g.Kind] || g.Reason == "" {
			t.Errorf("unexpected gap %+v", g)
		}
		delete(want, g.Kind)
	}
	if len(want) != 0 {
		t.Errorf("missing gaps %v", want)
	}
}

func TestBuildExecutionEvidenceIsFinalOnlyWhenTheExecutionIs(t *testing.T) {
	in := evidenceInput(t)
	in.Execution.State = ExecRunning
	b, err := BuildExecutionEvidence(in)
	if err != nil || b.Final {
		t.Fatalf("final %v, %v", b.Final, err)
	}
}

func TestExecutionEvidenceReportsMissingContext(t *testing.T) {
	for _, state := range []ExecState{ExecRunning, ExecSucceeded, ExecFailed, ExecCancelled} {
		for _, recorded := range []int{0, 1, 2} {
			in := evidenceInput(t)
			in.Execution.State = state
			if recorded == 0 {
				in.Contexts = nil
			} else if recorded == 2 {
				other := in.Contexts[0]
				other.StepID = in.Steps[1].ID
				in.Contexts = append(in.Contexts, other)
			}
			b, err := BuildExecutionEvidence(in)
			if err != nil {
				t.Fatal(err)
			}
			for _, step := range b.Steps {
				if (step.ContextGap != "") != (step.Context == nil) {
					t.Fatal("context gap does not identify the missing Step context")
				}
			}
			count := 0
			for _, gap := range b.Gaps {
				if gap.Kind == GapContext {
					count++
					if gap.Reason == "" {
						t.Fatal("unexplained context gap")
					}
				}
			}
			if (count == 1) != (recorded < 2) || count > 1 {
				t.Fatalf("state %s, recorded %d: context gaps %d", state, recorded, count)
			}
		}
	}
	in := evidenceInput(t)
	in.Steps, in.Contexts, in.Events, in.Approvals, in.Audit, in.Artifacts = nil, nil, nil, nil, nil, nil
	b, err := BuildExecutionEvidence(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range b.Gaps {
		if gap.Kind == GapContext {
			t.Fatal("Execution without Steps has a context gap")
		}
	}
}

func TestBuildExecutionEvidenceDigestIsStableAndSeesEveryPart(t *testing.T) {
	base, _ := BuildExecutionEvidence(evidenceInput(t))
	again, _ := BuildExecutionEvidence(evidenceInput(t))
	if base.Digest == "" || base.Digest != again.Digest {
		t.Fatalf("digest %q vs %q", base.Digest, again.Digest)
	}
	for name, edit := range map[string]func(*ExecutionEvidenceInput){
		"execution": func(in *ExecutionEvidenceInput) { in.Execution.State = ExecFailed },
		"step":      func(in *ExecutionEvidenceInput) { in.Steps[0].Reason = "x" },
		"event":     func(in *ExecutionEvidenceInput) { in.Events[0].Reason = "x" },
		"approval":  func(in *ExecutionEvidenceInput) { in.Approvals[0].VoidReason = "x" },
		"audit":     func(in *ExecutionEvidenceInput) { in.Audit[0].Reason = "x" },
		"context":   func(in *ExecutionEvidenceInput) { in.Contexts[0].Document = "other" },
		"artifact":  func(in *ExecutionEvidenceInput) { in.Artifacts[0].Hash = "sha256:x" },
	} {
		in := evidenceInput(t)
		edit(&in)
		b, err := BuildExecutionEvidence(in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if b.Digest == base.Digest {
			t.Errorf("a change to the %s did not change the digest", name)
		}
	}
}

func TestBuildExecutionEvidenceRefusesPartsOfAnotherRun(t *testing.T) {
	for name, edit := range map[string]func(*ExecutionEvidenceInput){
		"no execution id":   func(in *ExecutionEvidenceInput) { in.Execution.ID = "" },
		"foreign step":      func(in *ExecutionEvidenceInput) { in.Steps[1].ExecutionID = "e2" },
		"foreign event":     func(in *ExecutionEvidenceInput) { in.Events[0].ExecutionID = "e2" },
		"foreign approval":  func(in *ExecutionEvidenceInput) { in.Approvals[0].Gate.ExecutionID = "e2" },
		"foreign audit":     func(in *ExecutionEvidenceInput) { in.Audit[0].TargetID = "e2.a" },
		"audit of a grant":  func(in *ExecutionEvidenceInput) { in.Audit[0].TargetType = "grant" },
		"context no step":   func(in *ExecutionEvidenceInput) { in.Contexts[0].StepID = "nope" },
		"context other run": func(in *ExecutionEvidenceInput) { in.Contexts[0].ExecutionID = "e2" },
		"artifact no step":  func(in *ExecutionEvidenceInput) { in.Artifacts[3].StepID = "nope" },
		"artifact other":    func(in *ExecutionEvidenceInput) { in.Artifacts[0].ExecutionID = "e2" },
	} {
		in := evidenceInput(t)
		edit(&in)
		if _, err := BuildExecutionEvidence(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestBuildExecutionEvidenceRejectsUnknownStepReferences(t *testing.T) {
	for _, part := range []string{"event", "approval"} {
		t.Run(part, func(t *testing.T) {
			in := evidenceInput(t)
			if part == "event" {
				in.Events[0].StepID = "another-executions-step"
			} else {
				in.Approvals[0].Gate.StepID = "another-executions-step"
			}
			got, err := BuildExecutionEvidence(in)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v, want ErrInvalid", err)
			}
			if got.Digest != "" || got.Execution.ID != "" {
				t.Fatal("invalid attribution returned a bundle")
			}
		})
	}
}

func TestBuildExecutionEvidenceAllowsExecutionAndStepEvents(t *testing.T) {
	in := evidenceInput(t)
	in.Events[0].StepID = ""
	in.Events = append(in.Events, ExecutionEvent{ExecutionID: in.Execution.ID, Seq: 2, Kind: EventStepLeased, StepID: in.Steps[0].ID})
	got, err := BuildExecutionEvidence(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Events[0].StepID != "" || got.Events[1].StepID != in.Steps[0].ID || got.Digest == "" {
		t.Fatalf("events changed: %+v", got.Events)
	}
}

func TestBuildExecutionEvidenceRejectsAmbiguousSteps(t *testing.T) {
	for name, edit := range map[string]func(*ExecutionEvidenceInput){
		"empty step id": func(in *ExecutionEvidenceInput) {
			in.Steps = append(in.Steps, StepExecution{ExecutionID: in.Execution.ID})
		},
		"duplicate step": func(in *ExecutionEvidenceInput) {
			in.Steps = append(in.Steps, in.Steps[0])
		},
		"conflicting step": func(in *ExecutionEvidenceInput) {
			other := in.Steps[0]
			other.State = StepFailed
			in.Steps = append(in.Steps, other)
		},
		"duplicate context": func(in *ExecutionEvidenceInput) {
			in.Contexts = append(in.Contexts, in.Contexts[0])
		},
		"conflicting context": func(in *ExecutionEvidenceInput) {
			other := in.Contexts[0]
			other.Document = "different context"
			in.Contexts = append(in.Contexts, other)
		},
	} {
		t.Run(name, func(t *testing.T) {
			in := evidenceInput(t)
			edit(&in)
			b, err := BuildExecutionEvidence(in)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v, want ErrInvalid", err)
			}
			if !reflect.DeepEqual(b, ExecutionEvidence{}) {
				t.Fatal("ambiguous attribution returned a partial bundle")
			}
		})
	}
}
