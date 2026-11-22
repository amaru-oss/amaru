package httpapi

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
)

func executionEventsPath(key domain.ProjectKey, id, query string) string {
	return executionPath(key, id) + "/events" + query
}

// cancelA cancels projectA's Execution through the served command, journaling a real
// cancel_requested, two step.cancelled and one execution.cancelled entry.
func (env *executionEnv) cancelA(t *testing.T) {
	t.Helper()
	cookie, csrf := launch(t, env.auth, env.h)
	browser := map[string]string{"Cookie": cookie, "Origin": "http://localhost:7777", "X-Amaru-CSRF": csrf}
	check(t, callHost(env.h, "POST", executionPath(projectA, "exec-a")+"/cancel", `{"idempotencyKey":"cancel-1"}`, browser), 200, "")
}

// failB records a failed run of projectB's Execution directly in the store: a lease, a start,
// a restart retry (no public shape), the Step failure and the Execution failure.
func (env *executionEnv) failB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	e, steps, err := env.store.Get(ctx, "exec-b")
	if err != nil {
		t.Fatal(err)
	}
	ended := approvalNow.Add(4 * time.Second)
	st := steps[0]
	st.State, st.EndedAt = domain.StepFailed, &ended
	st.Error = &domain.ExecError{Code: "exit_1", Message: "push rejected"}
	e.State, e.EndedAt = domain.ExecFailed, &ended
	e.Error = &domain.ExecError{Code: "exit_1", Message: "push rejected"}
	at := func(s int) time.Time { return approvalNow.Add(time.Duration(s) * time.Second) }
	_, err = env.store.Commit(ctx, ports.ExecutionChange{Execution: &e, Steps: []domain.StepExecution{st}, Events: []domain.ExecutionEvent{
		{Kind: domain.EventStepLeased, StepID: st.ID, Reason: "worker-1", At: at(1)},
		{Kind: domain.EventStepStarted, StepID: st.ID, At: at(2)},
		{Kind: domain.EventStepRetried, StepID: st.ID, Reason: "platform_restarted", At: at(3)},
		{Kind: domain.EventStepFailed, StepID: st.ID, At: at(4)},
		{Kind: domain.EventExecFailed, At: at(4)},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExecutionEventsMatchSharedFixtures(t *testing.T) {
	env := executionSetup(t)
	w := call(env.events, env.auth, "GET", executionEventsPath(projectA, "exec-a", ""), "", nil)
	check(t, w, 200, "")
	if w.Body.String() != `{"events":[],"nextCursor":0}`+"\n" {
		t.Fatalf("empty journal: %s", w.Body)
	}
	env.cancelA(t)
	env.failB(t)
	w = call(env.events, env.auth, "GET", executionEventsPath(projectA, "exec-a", ""), "", nil)
	check(t, w, 200, "")
	sameFixture(t, w.Body.Bytes(), "../../events/v1/EventList/valid-platform-cancelled.json")
	w = call(env.events, env.auth, "GET", executionEventsPath(projectB, "exec-b", ""), "", nil)
	check(t, w, 200, "")
	sameFixture(t, w.Body.Bytes(), "../../events/v1/EventList/valid-platform-failed.json")
	// Reads are repeatable: the same ids for the same cursors.
	again := call(env.events, env.auth, "GET", executionEventsPath(projectB, "exec-b", ""), "", nil)
	if again.Body.String() != w.Body.String() {
		t.Fatalf("second read differs:\n%s", again.Body)
	}
}

func TestExecutionEventsPageByProjectCursor(t *testing.T) {
	env := executionSetup(t)
	env.failB(t)
	type page struct {
		Events []struct {
			Seq  int64
			Type string
		}
		NextCursor *int64
	}
	read := func(query string) page {
		t.Helper()
		w := call(env.events, env.auth, "GET", executionEventsPath(projectB, "exec-b", query), "", nil)
		check(t, w, 200, "")
		var p page
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || p.NextCursor == nil {
			t.Fatalf("page: %s", w.Body)
		}
		return p
	}
	first := read("?limit=2")
	if len(first.Events) != 2 || first.Events[1].Seq != 2 || *first.NextCursor != 2 {
		t.Fatalf("first page: %+v", first)
	}
	// The omitted retry entry (cursor 3) leaves a gap; the cursor moves past it.
	second := read("?limit=2&after=2")
	if len(second.Events) != 1 || second.Events[0].Seq != 4 || *second.NextCursor != 4 {
		t.Fatalf("second page: %+v", second)
	}
	rest := read("?after=4")
	if len(rest.Events) != 1 || rest.Events[0].Type != "execution.failed" || *rest.NextCursor != 5 {
		t.Fatalf("last page: %+v", rest)
	}
	if end := read("?after=5"); len(end.Events) != 0 || *end.NextCursor != 5 {
		t.Fatalf("end page: %+v", end)
	}
}

func TestExecutionEventsResync(t *testing.T) {
	env := executionSetup(t)
	env.failB(t)
	w := call(env.events, env.auth, "GET", executionEventsPath(projectB, "exec-b", "?after=99"), "", nil)
	check(t, w, 200, "")
	want := `{"events":[],"resync":{"type":"resync_required","reason":"cursor_ahead","resumeFrom":5}}` + "\n"
	if w.Body.String() != want {
		t.Fatalf("resync: %s", w.Body)
	}
}

func TestExecutionEventsAreProjectScoped(t *testing.T) {
	env := executionSetup(t)
	env.cancelA(t)
	env.failB(t)
	foreign := call(env.events, env.auth, "GET", executionEventsPath(projectA, "exec-b", ""), "", nil)
	missing := call(env.events, env.auth, "GET", executionEventsPath(projectA, "exec-z", ""), "", nil)
	check(t, foreign, 404, "not_found")
	if foreign.Body.String() != missing.Body.String() {
		t.Fatalf("foreign and missing differ:\n%s\n%s", foreign.Body, missing.Body)
	}
	w := call(env.events, env.auth, "GET", executionEventsPath(projectA, "exec-a", ""), "", nil)
	check(t, w, 200, "")
	if strings.Contains(w.Body.String(), "exec-b") || strings.Contains(w.Body.String(), string(projectB)) {
		t.Fatalf("other Project leaked: %s", w.Body)
	}
}

func TestExecutionEventsRefusals(t *testing.T) {
	env := executionSetup(t)
	for _, query := range []string{"?limit=0", "?limit=1001", "?limit=abc", "?after=-1", "?after=%2B1", "?limit=1&limit=2", "?after=", "?since=1", "?limit=1&state=running"} {
		check(t, call(env.events, env.auth, "GET", executionEventsPath(projectA, "exec-a", query), "", nil), 400, "invalid_request")
	}
	check(t, call(env.events, env.auth, "GET", executionEventsPath(projectA, "exec-a", "?limit=1000&after=0"), "", nil), 200, "")
	check(t, call(env.events, env.auth, "GET", executionEventsPath("id:01ARZ3NDEKTSV4RRFFQ69G5FAZ", "exec-a", ""), "", nil), 404, "not_found")
	check(t, call(env.events, nil, "GET", executionEventsPath(projectA, "exec-a", ""), "", nil), 401, "unauthorized")
	cookie, _ := launch(t, env.auth, env.h)
	if w := callHost(env.events, "GET", executionEventsPath(projectA, "exec-a", ""), "", map[string]string{"Cookie": cookie, "Host": "evil.example:7777"}); w.Code < 400 {
		t.Fatalf("foreign host accepted: %d", w.Code)
	}
}

func TestExecutionEventsRefuseUnrepresentable(t *testing.T) {
	for name, ev := range map[string]domain.ExecutionEvent{
		"unknown kind":             {Kind: "step.invented", StepID: "exec-b.push", At: approvalNow},
		"failure of a queued step": {Kind: domain.EventStepFailed, StepID: "exec-b.push", At: approvalNow},
		"foreign step":             {Kind: domain.EventStepStarted, StepID: "exec-a.push", At: approvalNow},
		"no time":                  {Kind: domain.EventStepStarted, StepID: "exec-b.push"},
	} {
		t.Run(name, func(t *testing.T) {
			env := executionSetup(t)
			ctx := context.Background()
			e, _, err := env.store.Get(ctx, "exec-b")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := env.store.Commit(ctx, ports.ExecutionChange{Execution: &e, Events: []domain.ExecutionEvent{ev}}); err != nil {
				t.Fatal(err)
			}
			w := call(env.events, env.auth, "GET", executionEventsPath(projectB, "exec-b", ""), "", nil)
			check(t, w, 500, "internal")
			if strings.Contains(w.Body.String(), "exec-b") {
				t.Fatalf("internal error leaked the id: %s", w.Body)
			}
		})
	}
}

func TestEventIDIsStableULID(t *testing.T) {
	ulid := regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)
	a, err := eventID("p", 1, approvalNow)
	if err != nil || !ulid.MatchString(a) {
		t.Fatalf("id %q: %v", a, err)
	}
	if again, _ := eventID("p", 1, approvalNow); again != a {
		t.Fatalf("not stable: %s %s", a, again)
	}
	for _, other := range [][2]any{{"p", int64(2)}, {"q", int64(1)}} {
		if id, _ := eventID(other[0].(string), other[1].(int64), approvalNow); id == a || id[:9] != a[:9] {
			t.Fatalf("%v: %s vs %s", other, id, a)
		}
	}
	if later, _ := eventID("p", 1, approvalNow.Add(time.Millisecond)); later <= a {
		t.Fatalf("time does not order ids: %s <= %s", later, a)
	}
	for _, bad := range []time.Time{{}, time.Unix(-1, 0)} {
		if _, err := eventID("p", 1, bad); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}
