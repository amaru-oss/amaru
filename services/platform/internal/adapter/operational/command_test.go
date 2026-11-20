package operational

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/coordinator/coordtest"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
)

type failingCommandStore struct {
	*fakes.ExecutionStore
	failure error
}

func (s failingCommandStore) Command(context.Context, string, string) (domain.CommandRecord, error) {
	return domain.CommandRecord{}, s.failure
}

func TestCommandFailureOmitsCredentialBearingError(t *testing.T) {
	secret := "fake-session-material"
	failure := errors.New("https://user:" + secret + "@provider.invalid/run?token=" + secret)
	store := failingCommandStore{fakes.NewExecutionStore(), failure}
	var buf bytes.Buffer
	logger, _ := New(&buf)
	commands := Commands{Next: coordinator.New(store, time.Now, coordtest.Permissive), Logger: logger}
	ctx := WithIdentity(context.Background(), Identity{InstanceID: "instance", CorrelationID: "command", ProjectID: "project"})
	_, _, err := commands.Submit(ctx, coordinator.SubmitRequest{Key: secret, ProjectID: "project", By: domain.Principal{ID: "human", Kind: domain.PrincipalHuman}, Steps: []coordinator.StepSpec{{Name: "test", Capability: "code.test"}}})
	if err != failure {
		t.Fatal("command error changed")
	}
	recs := records(t, &buf)
	if len(recs) != 1 || recs[0]["cause"] != "command dependency failed" || recs[0]["outcome"] != "failed" || strings.Contains(buf.String(), secret) || strings.Contains(buf.String(), "provider.invalid") {
		t.Fatal("unsafe or missing failure diagnostic")
	}
	commands.Logger, _ = New(panicWriter{})
	_, _, err = commands.Submit(ctx, coordinator.SubmitRequest{})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("sink panic changed rejection")
	}
}

func TestCommandSinkFailurePreservesCommitAndReplay(t *testing.T) {
	store := fakes.NewExecutionStore()
	logger, _ := New(panicWriter{})
	commands := Commands{Next: coordinator.New(store, time.Now, coordtest.Permissive), Logger: logger}
	ctx := WithIdentity(context.Background(), Identity{InstanceID: "instance", CorrelationID: "command"})
	req := coordinator.SubmitRequest{Key: "submit", ProjectID: "project", By: domain.Principal{ID: "human", Kind: domain.PrincipalHuman}, Steps: []coordinator.StepSpec{{Name: "test", Capability: "code.test"}}}
	out, replay, err := commands.Submit(ctx, req)
	if err != nil || replay {
		t.Fatal("sink failure changed successful command")
	}
	before, err := store.Events(ctx, out.Execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	again, replay, err := commands.Submit(ctx, req)
	if err != nil || !replay || again.Execution.ID != out.Execution.ID {
		t.Fatal("sink failure changed replay")
	}
	after, err := store.Events(ctx, out.Execution.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("replay diagnostics changed history")
	}
}
