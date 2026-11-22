package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
	"go.yaml.in/yaml/v3"
)

type executionEnv struct {
	auth   *localauth.Auth
	h      http.Handler
	events http.Handler
	store  *fakes.ExecutionStore
}

// executionSetup composes real compact auth, the authorized application boundary and fake
// stores. projectA runs a two-step Workflow whose git.push Step waits for an Approval; projectB
// runs one git.push Step submitted without a Workflow.
func executionSetup(t *testing.T) *executionEnv {
	t.Helper()
	ctx := context.Background()
	env := &executionEnv{store: fakes.NewExecutionStore()}
	auth, err := localauth.New(&tcpLoopback)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auth.Close)
	env.auth = auth
	projects := fakes.NewProjectStore()
	trust := fakes.NewTrustStore(projects)
	fakes.LinkTrustAudit(env.store, trust)
	var executions, approvals atomic.Int64
	c := coordinator.New(env.store, func() time.Time { return approvalNow }, coordinator.WithTrust(trust),
		coordinator.WithExecutionIDs(func() string { return []string{"exec-a", "exec-b"}[executions.Add(1)-1] }),
		coordinator.WithApprovalIDs(func() string {
			return []string{"01JD7Q0000000000000000000A", "01JD7Q0000000000000000000B"}[approvals.Add(1)-1]
		}))
	owner := domain.Principal{Kind: domain.PrincipalHuman, ID: "local-administrator", Role: domain.RoleOwner}
	for _, key := range []domain.ProjectKey{projectA, projectB} {
		if err := projects.Create(ctx, domain.Project{Key: key, Name: "same", CreatedAt: approvalNow}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.TrustContent(ctx, "trust-"+string(key), owner, key, domain.TrustProject, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	seed, err := coordinator.NewProjectAccess(c, projects, func(context.Context) (domain.Principal, error) { return owner, nil })
	if err != nil {
		t.Fatal(err)
	}
	workflow := domain.Workflow{ID: "release", Name: "Release", Status: domain.WorkflowActive, Steps: []domain.WorkflowStep{
		{ID: "test", Name: "Run tests", Type: domain.StepTypeTool, Capability: "code.test"},
		{ID: "push", Name: "Push branch", Type: domain.StepTypeTool, Capability: "git.push"},
	}}
	pin, err := domain.Pin(workflow, "")
	if err != nil {
		t.Fatal(err)
	}
	// Explicit test-owner trust is a precondition; default Policy remains active.
	if _, err := c.TrustContent(ctx, "trust-workflow", owner, projectA, domain.TrustWorkflow, workflow.ID, pin.Hash); err != nil {
		t.Fatal(err)
	}
	if _, _, err := seed.Submit(ctx, projectA, coordinator.SubmitRequest{Key: "submit-a", Workflow: &workflow}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := seed.Submit(ctx, projectB, coordinator.SubmitRequest{Key: "submit-b", Steps: []coordinator.StepSpec{{Name: "push", Capability: "git.push"}}}); err != nil {
		t.Fatal(err)
	}
	// The HTTP boundary uses the compact session identity, not the seeding callback.
	access, err := coordinator.NewProjectAccess(c, projects, auth.Principal)
	if err != nil {
		t.Fatal(err)
	}
	if env.h, err = ExecutionsHandler(auth, access); err != nil {
		t.Fatal(err)
	}
	if env.events, err = ExecutionEventsHandler(auth, access, env.store, domain.CompactReplayWindow); err != nil {
		t.Fatal(err)
	}
	return env
}

func executionPath(key domain.ProjectKey, id string) string {
	return "/v1/projects/" + url.PathEscape(string(key)) + "/executions/" + url.PathEscape(id)
}

func (env *executionEnv) snapshot(t *testing.T) string {
	t.Helper()
	var all []any
	for _, id := range []string{"exec-a", "exec-b"} {
		e, steps, err := env.store.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, e, steps)
	}
	b, _ := json.Marshal(all)
	return string(b)
}

func sameFixture(t *testing.T, body []byte, fixture string) {
	t.Helper()
	data, err := os.ReadFile("../../../../../packages/contracts/fixtures/api/v1/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if json.Unmarshal(body, &got) != nil || json.Unmarshal(data, &want) != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("response differs from %s:\n%s", fixture, body)
	}
}

func TestExecutionGetMatchesSharedFixtures(t *testing.T) {
	env := executionSetup(t)
	w := call(env.h, env.auth, "GET", executionPath(projectA, "exec-a"), "", nil)
	check(t, w, 200, "")
	sameFixture(t, w.Body.Bytes(), "Execution/valid-platform-workflow.json")
	w = call(env.h, env.auth, "GET", executionPath(projectB, "exec-b"), "", nil)
	check(t, w, 200, "")
	sameFixture(t, w.Body.Bytes(), "Execution/valid-platform-single-step.json")
}

func TestExecutionForeignAndMissingAreIdentical(t *testing.T) {
	env := executionSetup(t)
	before := env.snapshot(t)
	var bodies []string
	for _, target := range []string{executionPath(projectB, "exec-a"), executionPath(projectA, "exec-missing")} {
		w := call(env.h, env.auth, "GET", target, "", nil)
		check(t, w, 404, "not_found")
		bodies = append(bodies, w.Body.String())
		w = call(env.h, env.auth, "POST", target+"/cancel", `{"idempotencyKey":"k-foreign"}`, nil)
		check(t, w, 404, "not_found")
		bodies = append(bodies, w.Body.String())
	}
	for _, b := range bodies[1:] {
		if b != bodies[0] {
			t.Fatal("foreign and missing responses differ")
		}
	}
	if env.snapshot(t) != before {
		t.Fatal("refused cancel mutated an Execution")
	}
}

func TestExecutionCancelAndReplay(t *testing.T) {
	env := executionSetup(t)
	cookie, csrf := launch(t, env.auth, env.h)
	browser := map[string]string{"Cookie": cookie, "Origin": "http://localhost:7777", "X-Amaru-CSRF": csrf}
	target := executionPath(projectA, "exec-a") + "/cancel"
	body := `{"idempotencyKey":"cancel-1","reason":"wrong branch"}`
	first := callHost(env.h, "POST", target, body, browser)
	check(t, first, 200, "")
	var got struct {
		State string
		Steps []struct{ StepID, State, EndedAt string }
	}
	if err := json.Unmarshal(first.Body.Bytes(), &got); err != nil || got.State != "cancelled" || len(got.Steps) != 2 {
		t.Fatalf("cancel: %s", first.Body)
	}
	for _, s := range got.Steps {
		if s.State != "cancelled" || s.EndedAt == "" {
			t.Fatalf("step not ended: %s", first.Body)
		}
	}
	after := env.snapshot(t)
	// The same key and body replays the recorded command without another write.
	replay := callHost(env.h, "POST", target, body, browser)
	check(t, replay, 200, "")
	if replay.Body.String() != first.Body.String() || env.snapshot(t) != after {
		t.Fatalf("replay differs or wrote:\n%s", replay.Body)
	}
	// The same key with another request, and a new cancel of an ended Execution, conflict.
	for _, b := range []string{`{"idempotencyKey":"cancel-1","reason":"other"}`, `{"idempotencyKey":"cancel-2"}`} {
		check(t, callHost(env.h, "POST", target, b, browser), 409, "conflict")
	}
	if env.snapshot(t) != after {
		t.Fatal("refused cancel wrote")
	}
	// The other Project's Execution is untouched.
	w := call(env.h, env.auth, "GET", executionPath(projectB, "exec-b"), "", nil)
	check(t, w, 200, "")
	sameFixture(t, w.Body.Bytes(), "Execution/valid-platform-single-step.json")
}

func TestExecutionCancelRefusals(t *testing.T) {
	env := executionSetup(t)
	before := env.snapshot(t)
	target := executionPath(projectA, "exec-a") + "/cancel"
	long := strings.Repeat("r", maxCancelReason+1)
	for name, body := range map[string]string{
		"unknown field":   `{"idempotencyKey":"k","actor":"someone"}`,
		"no key":          `{"reason":"x"}`,
		"empty key":       `{"idempotencyKey":""}`,
		"long key":        `{"idempotencyKey":"` + strings.Repeat("k", 129) + `"}`,
		"long reason":     `{"idempotencyKey":"k","reason":"` + long + `"}`,
		"two documents":   `{"idempotencyKey":"k"}{"idempotencyKey":"k"}`,
		"not an object":   `[]`,
		"oversized":       `{"idempotencyKey":"k","reason":"` + strings.Repeat("r", maxCancelBody) + `"}`,
		"malformed":       `{"idempotencyKey":`,
		"trailing value":  `{"idempotencyKey":"k"} 1`,
		"key not string":  `{"idempotencyKey":1}`,
		"null key":        `{"idempotencyKey":null}`,
		"reason not text": `{"idempotencyKey":"k","reason":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			check(t, call(env.h, env.auth, "POST", target, body, nil), 400, "invalid_request")
		})
	}
	check(t, call(env.h, env.auth, "POST", target, `{"idempotencyKey":"k"}`, map[string]string{"Content-Type": "text/plain"}), 400, "invalid_request")
	check(t, call(env.h, nil, "POST", target, `{"idempotencyKey":"k"}`, nil), 401, "unauthorized")
	check(t, call(env.h, nil, "GET", executionPath(projectA, "exec-a"), "", nil), 401, "unauthorized")
	if env.snapshot(t) != before {
		t.Fatal("refused cancel mutated an Execution")
	}
}

func TestExecutionBrowserGuards(t *testing.T) {
	env := executionSetup(t)
	before := env.snapshot(t)
	cookie, csrf := launch(t, env.auth, env.h)
	target := executionPath(projectA, "exec-a") + "/cancel"
	body := `{"idempotencyKey":"k"}`
	for name, headers := range map[string]map[string]string{
		"no csrf":        {"Cookie": cookie, "Origin": "http://localhost:7777"},
		"foreign origin": {"Cookie": cookie, "Origin": "http://evil.example", "X-Amaru-CSRF": csrf},
		"foreign host":   {"Cookie": cookie, "Origin": "http://localhost:7777", "X-Amaru-CSRF": csrf, "Host": "evil.example:7777"},
	} {
		t.Run(name, func(t *testing.T) {
			if w := callHost(env.h, "POST", target, body, headers); w.Code < 400 {
				t.Fatalf("accepted: %d %s", w.Code, w.Body)
			}
		})
	}
	if env.snapshot(t) != before {
		t.Fatal("guarded request mutated an Execution")
	}
}

func TestExecutionSharedCancelFixtures(t *testing.T) {
	files, err := filepath.Glob("../../../../../packages/contracts/fixtures/api/v1/CancelExecution/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			env := executionSetup(t)
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			w := call(env.h, env.auth, "POST", executionPath(projectA, "exec-a")+"/cancel", string(data), nil)
			if strings.HasPrefix(filepath.Base(file), "valid") {
				check(t, w, 200, "")
			} else {
				check(t, w, 400, "invalid_request")
			}
		})
	}
}

func TestExecutionRoutesMatchOpenAPI(t *testing.T) {
	data, err := os.ReadFile("../../../../../packages/contracts/openapi/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	served := append(slices.Clone(executionPatterns), executionEventPatterns...)
	for _, pattern := range served {
		method, target, _ := strings.Cut(pattern, " ")
		if _, ok := doc.Paths[strings.TrimPrefix(target, "/v1")][strings.ToLower(method)]; !ok {
			t.Fatalf("undocumented route: %s", pattern)
		}
	}
	// The documented Execution operation not served yet: create needs Agent, Skill and
	// Environment resolution.
	pending := map[string]bool{
		"POST /v1/projects/{projectId}/executions": true,
	}
	for path, ops := range doc.Paths {
		if !strings.HasPrefix(path, "/projects/{projectId}/executions") {
			continue
		}
		for method := range ops {
			if method == "parameters" {
				continue
			}
			route := strings.ToUpper(method) + " /v1" + path
			isServed := slices.Contains(served, route)
			if isServed == pending[route] {
				t.Fatalf("Execution route inventory drifted: %s served=%v", route, isServed)
			}
		}
	}
}

func TestWireExecutionRefusesUnrepresentable(t *testing.T) {
	now := approvalNow
	ok := domain.Execution{ID: "e", ProjectID: "p", State: domain.ExecRunning, CreatedAt: now, StartedAt: &now}
	step := domain.StepExecution{ID: "e.s", ExecutionID: "e", State: domain.StepRunning, Capability: "code.test", StartedAt: &now}
	if _, err := wireExecution(ok, []domain.StepExecution{step}); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	for name, mutate := range map[string]func(*domain.Execution, *domain.StepExecution){
		"failed without error":  func(e *domain.Execution, _ *domain.StepExecution) { e.State, e.EndedAt = domain.ExecFailed, &now },
		"ended without endedAt": func(e *domain.Execution, _ *domain.StepExecution) { e.State = domain.ExecSucceeded },
		"queued but started":    func(e *domain.Execution, _ *domain.StepExecution) { e.State = domain.ExecQueued },
		"unknown state":         func(e *domain.Execution, _ *domain.StepExecution) { e.State = "paused" },
		"foreign step":          func(_ *domain.Execution, s *domain.StepExecution) { s.ID = "other.s" },
		"step state":            func(_ *domain.Execution, s *domain.StepExecution) { s.State = "paused" },
		"refused without reason": func(_ *domain.Execution, s *domain.StepExecution) {
			s.State, s.EndedAt = domain.StepRefused, &now
		},
		"step ended without end": func(_ *domain.Execution, s *domain.StepExecution) { s.State = domain.StepSucceeded },
		"step failed no error": func(_ *domain.Execution, s *domain.StepExecution) {
			s.State, s.EndedAt = domain.StepFailed, &now
		},
		"bad capability": func(_ *domain.Execution, s *domain.StepExecution) { s.Capability = "Push" },
		"agent snapshot": func(_ *domain.Execution, s *domain.StepExecution) { s.Agent = &domain.ResolvedAgent{ID: "a"} },
		"step not in definition": func(e *domain.Execution, _ *domain.StepExecution) {
			e.Definition = &domain.DefinitionPin{WorkflowID: "w", Snapshot: json.RawMessage(`{"steps":[{"id":"x","name":"X","type":"tool"}]}`)}
		},
		"incomplete error": func(e *domain.Execution, _ *domain.StepExecution) {
			e.State, e.EndedAt, e.Error = domain.ExecFailed, &now, &domain.ExecError{Code: "x"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e, s := ok, step
			mutate(&e, &s)
			if _, err := wireExecution(e, []domain.StepExecution{s}); !errors.Is(err, errUnrepresentable) {
				t.Fatalf("mapped: %v", err)
			}
		})
	}
}

func executionListPath(key domain.ProjectKey, query string) string {
	return "/v1/projects/" + url.PathEscape(string(key)) + "/executions" + query
}

func TestExecutionListMatchesSharedFixture(t *testing.T) {
	env := executionSetup(t)
	w := call(env.h, env.auth, "GET", executionListPath(projectA, ""), "", nil)
	check(t, w, 200, "")
	sameFixture(t, w.Body.Bytes(), "ExecutionList/valid-platform.json")
}

func TestExecutionListIsProjectScopedAndFollowsCancel(t *testing.T) {
	env := executionSetup(t)
	list := func(key domain.ProjectKey) []struct{ ID, ProjectID, State string } {
		t.Helper()
		w := call(env.h, env.auth, "GET", executionListPath(key, ""), "", nil)
		check(t, w, 200, "")
		var out []struct{ ID, ProjectID, State string }
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if b := list(projectB); len(b) != 1 || b[0].ID != "exec-b" || b[0].ProjectID != string(projectB) {
		t.Fatalf("projectB: %+v", b)
	}
	check(t, call(env.h, env.auth, "POST", executionPath(projectA, "exec-a")+"/cancel", `{"idempotencyKey":"c"}`, nil), 200, "")
	if a := list(projectA); len(a) != 1 || a[0].ID != "exec-a" || a[0].State != "cancelled" {
		t.Fatalf("projectA after cancel: %+v", a)
	}
	if b := list(projectB); len(b) != 1 || b[0].State != "queued" {
		t.Fatalf("projectB changed: %+v", b)
	}
}

func TestExecutionListRefusals(t *testing.T) {
	env := executionSetup(t)
	for _, query := range []string{"?state=running", "?limit=1", "?"} {
		w := call(env.h, env.auth, "GET", executionListPath(projectA, query), "", nil)
		if query == "?" {
			// An empty query string carries no parameter.
			check(t, w, 200, "")
			continue
		}
		check(t, w, 400, "invalid_request")
	}
	check(t, call(env.h, env.auth, "GET", executionListPath("id:01ARZ3NDEKTSV4RRFFQ69G5FAZ", ""), "", nil), 404, "not_found")
	check(t, call(env.h, nil, "GET", executionListPath(projectA, ""), "", nil), 401, "unauthorized")
	cookie, _ := launch(t, env.auth, env.h)
	if w := callHost(env.h, "GET", executionListPath(projectA, ""), "", map[string]string{"Cookie": cookie, "Host": "evil.example:7777"}); w.Code < 400 {
		t.Fatalf("foreign host accepted: %d", w.Code)
	}
}

func TestExecutionListRefusesUnrepresentable(t *testing.T) {
	env := executionSetup(t)
	ctx := context.Background()
	e, _, err := env.store.Get(ctx, "exec-a")
	if err != nil {
		t.Fatal(err)
	}
	// A failed Execution without its error cannot be described; the store accepts the row.
	ended := approvalNow
	e.State, e.EndedAt = domain.ExecFailed, &ended
	if _, err := env.store.Commit(ctx, ports.ExecutionChange{Execution: &e}); err != nil {
		t.Fatal(err)
	}
	w := call(env.h, env.auth, "GET", executionListPath(projectA, ""), "", nil)
	check(t, w, 500, "internal")
	if strings.Contains(w.Body.String(), "exec-a") {
		t.Fatalf("internal error leaked the id: %s", w.Body)
	}
}
