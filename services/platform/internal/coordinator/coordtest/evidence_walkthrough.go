package coordtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/rluders/amaru/services/platform/internal/artifact"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/evidence"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
)

// EvidenceWalkthrough composes existing records; fake Worker results do not ingest outputs.
func EvidenceWalkthrough(t *testing.T, newStores func(*testing.T) SkillStores, nl NewLink) {
	ctx := context.Background()
	st := newStores(t)
	maintainer := domain.Principal{Kind: domain.PrincipalHuman, ID: "maint-1", Role: domain.RoleMaintainer}
	skill, err := domain.NewSkill(clock(), maintainer, domain.SkillOrganization, "", "review", "1", "Read the test output.")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Skills.Add(ctx, skill); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int64
	c := coordinator.New(st.Executions, clock, NewOpenTrust().Option(), coordinator.WithSkills(st.Skills), coordinator.WithContexts(st.Contexts),
		coordinator.WithAttemptIDs(func() string { return fmt.Sprintf("att-%d", attempts.Add(1)) }))
	got, _, err := c.Submit(ctx, coordinator.SubmitRequest{Key: "walkthrough", By: Requester, ProjectID: "p1", EnvironmentID: "env1",
		Steps: []coordinator.StepSpec{
			{Name: "test", Capability: "code.test", Inputs: []byte(`{"pkg":"./..."}`), Skills: []domain.SkillRef{{Name: skill.Name, Version: skill.Version, Hash: skill.Hash}}},
			{Name: "push", Capability: "git.push", Inputs: []byte(`{"branch":"review"}`)},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 2 || len(got.Approvals) != 1 || got.Steps[1].State != domain.StepAwaitingApproval {
		t.Fatalf("submission: %+v", got)
	}
	assembled, err := c.AssembleContext(ctx, coordinator.ContextRequest{ExecutionID: got.Execution.ID, StepID: got.Steps[0].ID, Intent: "Inspect the test output."})
	if err != nil {
		t.Fatal(err)
	}
	index := fakes.NewArtifactIndex()
	reader := evidence.Reader{Executions: st.Executions, Contexts: st.Contexts, Artifacts: index}
	bundle := func() domain.ExecutionEvidence {
		t.Helper()
		b, err := reader.Bundle(ctx, got.Execution.ID)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	initial := bundle()
	if initial.Final || initial.Steps[0].UsageGap == "" || initial.Steps[1].Context != nil || len(initial.Audit) != 1 {
		t.Fatalf("initial partial evidence: %+v", initial)
	}
	s := startSession(t, st.Executions, c, nl, "fake-worker")
	s.send(ports.Offer{FreeSlots: 1})
	first := s.assignment()
	if first.ExecutionID != got.Execution.ID || first.StepID != got.Steps[0].ID {
		t.Fatalf("first assignment: %+v", first)
	}
	zero := uint64(0)
	result := ports.Result{AttemptID: first.AttemptID, FencingToken: first.Lease.FencingToken, Outcome: ports.OutcomeSucceeded,
		Usage: []ports.UsageReport{{Provider: "fake", Model: "test-model", InputTokens: &ports.IntMeasurement{Source: ports.UsageReported, Value: &zero}, OutputTokens: &ports.IntMeasurement{Source: ports.UsageUnknown}, Cost: &ports.Cost{Source: ports.UsageEstimated, Amount: "0.001230", Currency: "USD", PriceTableVersion: "fixture-v1"}}}}
	s.send(ports.AcceptAttempt{AttemptID: first.AttemptID, FencingToken: first.Lease.FencingToken})
	ack := func(want ports.ResultDisposition, attempt string) {
		t.Helper()
		m, ok := s.recv().(ports.ResultAck)
		if !ok || m.Disposition != want || m.AttemptID != attempt {
			t.Fatalf("result acknowledgement: %+v, want %v for %s", m, want, attempt)
		}
	}
	s.send(result)
	ack(ports.ResultAccepted, first.AttemptID)

	// Outputs have no Worker Result field; ingest fixture files explicitly through the existing adapter.
	location := t.TempDir()
	const content = "fixture test output\n"
	if err := os.WriteFile(filepath.Join(location, "output.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	ingester := artifact.Ingester{Blobs: fakes.NewBlobStore(), Index: index, Now: clock}
	versions, err := ingester.Ingest(ctx, artifact.Source{ExecutionID: first.ExecutionID, StepID: first.StepID, AttemptID: first.AttemptID}, location,
		[]artifact.Output{{ArtifactID: "z-output", Path: "output.txt", MediaType: "text/plain"}, {ArtifactID: "a-output", Path: "output.txt", MediaType: "text/plain"}})
	if err != nil {
		t.Fatal(err)
	}
	partial := bundle()
	if partial.Final || partial.Steps[0].State != domain.StepSucceeded || partial.Steps[1].State != domain.StepAwaitingApproval ||
		partial.Steps[0].UsageGap != "" || partial.Steps[1].UsageGap == "" {
		t.Fatalf("partial evidence: %+v", partial)
	}
	s.send(ports.Offer{FreeSlots: 1})
	s.expectNothing()
	a := got.Approvals[0]
	// Leave the gated Step's context absent: assembling it now would change its Approval Evidence.
	decision, _, err := c.DecideApproval(ctx, coordinator.DecideRequest{Key: "review", ApprovalID: a.ID, By: maintainer, Outcome: domain.OutcomeApproved, Rationale: "fixture reviewed", EvidenceDigest: a.Evidence.Digest})
	if err != nil {
		t.Fatal(err)
	}
	second := s.assignment()
	if second.ExecutionID != first.ExecutionID || second.StepID != got.Steps[1].ID || second.AttemptID == first.AttemptID {
		t.Fatalf("second assignment: %+v", second)
	}
	s.send(ports.AcceptAttempt{AttemptID: second.AttemptID, FencingToken: second.Lease.FencingToken})
	s.send(ports.Result{AttemptID: second.AttemptID, FencingToken: second.Lease.FencingToken, Outcome: ports.OutcomeFailed, ErrorCode: "fixture_failed", ErrorDetail: "fixture failure"})
	ack(ports.ResultAccepted, second.AttemptID)
	endedSteps := bundle()
	// This #19 base does not aggregate normal Step completion into terminal Execution state.
	if endedSteps.Final || endedSteps.Execution.State != domain.ExecRunning {
		t.Fatalf("unexpected Execution completion semantics: %+v", endedSteps.Execution)
	}
	cancelled, _, err := c.Cancel(ctx, coordinator.CancelRequest{Key: "finish-walkthrough", ExecutionID: got.Execution.ID, By: domain.Actor{Kind: domain.ActorHuman}, ActorID: maintainer.ID, Reason: "end fixture walkthrough"})
	if err != nil || !cancelled.ExecutionCancelled {
		t.Fatalf("explicit cancellation: %+v, %v", cancelled, err)
	}
	final := bundle()
	if !final.Final || final.Execution.State != domain.ExecCancelled || final.Execution.EndedAt == nil || final.Execution.RequestedBy != Requester {
		t.Fatalf("terminal evidence: %+v", final.Execution)
	}
	if len(final.Steps) != 2 || final.Steps[0].ID != first.StepID || final.Steps[1].ID != second.StepID ||
		final.Steps[0].State != domain.StepSucceeded || final.Steps[1].State != domain.StepFailed || final.Steps[1].Error == nil || final.Steps[1].Error.Code != "fixture_failed" {
		t.Fatalf("Step order/outcomes: %+v", final.Steps)
	}
	for i, step := range final.Steps {
		if step.InputsHash != domain.ContentHash(got.Steps[i].InputsJSON) || step.StartedAt == nil || step.EndedAt == nil || len(step.Usage) != 1 {
			t.Fatalf("Step evidence: %+v", step)
		}
		capture := step.Usage[0]
		if capture.ExecutionID != final.Execution.ID || capture.ProjectID != "p1" || capture.EnvironmentID != "env1" || capture.StepID != step.ID || capture.AttemptID != step.AttemptID || capture.FenceToken != 1 || !capture.CapturedAt.Equal(clock()) {
			t.Fatalf("Usage attribution: %+v", capture)
		}
	}
	if !reflect.DeepEqual(final.Steps[0].Usage[0].Reports, result.Usage) || final.Steps[0].UsageGap != "" || len(final.Steps[1].Usage[0].Reports) != 0 ||
		final.Steps[1].UsageGap != "accepted Worker result contains no usage reports; usage unknown" {
		t.Fatal("reported zero, unknown measurement, precise cost or empty-result gap lost")
	}
	if !reflect.DeepEqual(final.Steps[0].Context, &assembled) || !reflect.DeepEqual(final.Steps[0].Skills, got.Steps[0].Skills) || final.Steps[1].Context != nil ||
		final.Steps[0].ContextGap != "" || final.Steps[1].ContextGap == "" {
		t.Fatal("context/Skill attribution lost or absent context fabricated")
	}
	if !reflect.DeepEqual(final.Steps[0].Artifacts, []domain.ArtifactVersion{versions[1], versions[0]}) || len(final.Steps[1].Artifacts) != 0 {
		t.Fatal("Artifact grouping or ordering lost")
	}
	for _, v := range final.Steps[0].Artifacts {
		if v.Hash != domain.ContentHash([]byte(content)) || v.Size != int64(len(content)) || v.ExecutionID != first.ExecutionID || v.StepID != first.StepID || v.AttemptID != first.AttemptID || v.Version != 1 {
			t.Fatalf("Artifact attribution/hash: %+v", v)
		}
	}
	if !reflect.DeepEqual(final.Approvals, []domain.Approval{decision.Approval}) || final.Approvals[0].Gate.StepID != second.StepID || final.Approvals[0].Requester != Requester ||
		decision.Approval.Decision == nil || decision.Approval.Decision.Approver != maintainer.ID || decision.Approval.Decision.EvidenceDigest != a.Evidence.Digest {
		t.Fatalf("Approval attribution: %+v", final.Approvals)
	}
	if len(final.Audit) != 2 || final.Audit[0].Kind != domain.AuditApprovalRequested || final.Audit[1].Kind != domain.AuditApprovalApproved ||
		final.Audit[1].Actor != maintainer.ID || final.Audit[1].ActorKind != domain.ActorHuman || final.Audit[1].Reason != "fixture reviewed" || final.Audit[0].Seq >= final.Audit[1].Seq {
		t.Fatalf("ordered Approval Audit: %+v", final.Audit)
	}
	for _, ev := range final.Audit {
		if ev.TargetType != "approval" || ev.TargetID != a.ID {
			t.Fatalf("unrelated Audit entry: %+v", ev)
		}
	}
	var kinds []string
	for i, ev := range final.Events {
		if ev.ExecutionID != final.Execution.ID || ev.Seq <= 0 || i > 0 && ev.Seq <= final.Events[i-1].Seq || ev.StepID != "" && ev.StepID != first.StepID && ev.StepID != second.StepID {
			t.Fatalf("Event order/attribution: %+v", ev)
		}
		kinds = append(kinds, ev.Kind)
	}
	want := []string{domain.EventStepLeased, domain.EventStepStarted, domain.EventStepSucceeded, domain.EventApprovalApproved, domain.EventStepLeased, domain.EventStepStarted, domain.EventStepFailed, domain.EventExecCancelRequested, domain.EventExecCancelled}
	if !slices.Equal(kinds, want) {
		t.Fatalf("Event history: %v, want %v", kinds, want)
	}
	var gaps []string
	for _, gap := range final.Gaps {
		if gap.Reason == "" {
			t.Fatal("unexplained gap")
		}
		gaps = append(gaps, gap.Kind)
	}
	if !slices.Equal(gaps, []string{domain.GapCheck, domain.GapDiff, domain.GapLog, domain.GapWorkspace, domain.GapUsage, domain.GapContext}) {
		t.Fatalf("explicit recorder gaps: %v", gaps)
	}
	s.send(result)
	ack(ports.ResultDuplicate, first.AttemptID)
	if again := bundle(); final.Digest == "" || !reflect.DeepEqual(final, again) {
		t.Fatal("unchanged/replayed evidence changed the terminal bundle")
	}
	missing, err := reader.Bundle(ctx, "missing")
	if !errors.Is(err, domain.ErrNotFound) || !reflect.DeepEqual(missing, domain.ExecutionEvidence{}) {
		t.Fatalf("missing Execution: %+v, %v", missing, err)
	}
}
