package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
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

const fixtureApprovalID = "01JD7Q0000000000000000000A"

var approvalNow = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

type approvalEnv struct {
	auth    *localauth.Auth
	h       http.Handler
	store   *fakes.ExecutionStore
	access  *coordinator.ProjectAccess
	pending domain.Approval
}

// approvalSetup composes real compact auth, the authorized application boundary and fake stores.
// One git.push Step in projectA waits for a human Approval; projectB holds a second one.
func approvalSetup(t *testing.T) *approvalEnv {
	t.Helper()
	ctx := context.Background()
	env := &approvalEnv{store: fakes.NewExecutionStore()}
	auth, err := localauth.New(&tcpLoopback)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auth.Close)
	env.auth = auth
	projects := fakes.NewProjectStore()
	trust := fakes.NewTrustStore(projects)
	fakes.LinkTrustAudit(env.store, trust)
	approvalIDs := []string{fixtureApprovalID, "01JD7Q0000000000000000000B"}
	var executions, approvals atomic.Int64
	c := coordinator.New(env.store, func() time.Time { return approvalNow }, coordinator.WithTrust(trust),
		coordinator.WithExecutionIDs(func() string { return []string{"exec-a", "exec-b"}[executions.Add(1)-1] }),
		coordinator.WithApprovalIDs(func() string { return approvalIDs[approvals.Add(1)-1] }))
	owner := domain.Principal{Kind: domain.PrincipalHuman, ID: "local-administrator", Role: domain.RoleOwner}
	for _, key := range []domain.ProjectKey{projectA, projectB} {
		if err := projects.Create(ctx, domain.Project{Key: key, Name: "same", CreatedAt: approvalNow}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.TrustContent(ctx, "trust-"+string(key), owner, key, domain.TrustProject, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	env.access, err = coordinator.NewProjectAccess(c, projects, func(context.Context) (domain.Principal, error) { return owner, nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []domain.ProjectKey{projectA, projectB} {
		out, _, err := env.access.Submit(ctx, key, coordinator.SubmitRequest{Key: "submit-" + string(key), Steps: []coordinator.StepSpec{{Name: "push", Capability: "git.push"}}})
		if err != nil || len(out.Approvals) != 1 {
			t.Fatalf("seed %s: %+v %v", key, out, err)
		}
		if key == projectA {
			env.pending = out.Approvals[0]
		}
	}
	// The HTTP boundary uses the compact session identity, not the seeding callback.
	env.access, err = coordinator.NewProjectAccess(c, projects, auth.Principal)
	if err != nil {
		t.Fatal(err)
	}
	env.h, err = ApprovalsHandler(auth, env.access)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

var tcpLoopback = net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7777}

// callHost sends a request without the bearer credential; a "Host" header entry replaces the
// request host.
func callHost(h http.Handler, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost:7777"+target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if k == "Host" {
			r.Host = v
			continue
		}
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func approvalPath(key domain.ProjectKey, id string) string {
	return "/v1/projects/" + url.PathEscape(string(key)) + "/approvals/" + url.PathEscape(id)
}

func decisionBody(outcome, digest, rationale, key string) string {
	b, _ := json.Marshal(map[string]string{"outcome": outcome, "evidenceDigest": digest, "rationale": rationale, "idempotencyKey": key})
	return string(b)
}

func (env *approvalEnv) snapshot(t *testing.T) string {
	t.Helper()
	ap, err := env.store.Approval(context.Background(), env.pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(ap)
	return string(b)
}

func TestApprovalGetMatchesSharedFixture(t *testing.T) {
	env := approvalSetup(t)
	if env.pending.ID != fixtureApprovalID {
		t.Fatalf("seed id %s", env.pending.ID)
	}
	w := call(env.h, env.auth, "GET", approvalPath(projectA, env.pending.ID), "", nil)
	check(t, w, 200, "")
	data, err := os.ReadFile("../../../../../packages/contracts/fixtures/api/v1/Approval/valid-platform-requested.json")
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || json.Unmarshal(data, &want) != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("response differs from the shared fixture:\n%s", w.Body)
	}
}

func TestApprovalForeignAndMissingAreIdentical(t *testing.T) {
	env := approvalSetup(t)
	before := env.snapshot(t)
	var bodies []string
	for _, target := range []string{approvalPath(projectB, env.pending.ID), approvalPath(projectA, "01JD7Q0000000000000000000Z")} {
		w := call(env.h, env.auth, "GET", target, "", nil)
		check(t, w, 404, "not_found")
		bodies = append(bodies, w.Body.String())
		w = call(env.h, env.auth, "POST", target+"/decision", decisionBody("approved", env.pending.Evidence.Digest, "", "k-foreign"), nil)
		check(t, w, 404, "not_found")
		bodies = append(bodies, w.Body.String())
	}
	for _, b := range bodies[1:] {
		if b != bodies[0] {
			t.Fatal("foreign and missing responses differ")
		}
	}
	if env.snapshot(t) != before {
		t.Fatal("refused decision mutated the Approval")
	}
}

func TestApprovalDecisionApproveAndReplay(t *testing.T) {
	env := approvalSetup(t)
	cookie, csrf := launch(t, env.auth, env.h)
	browser := map[string]string{"Cookie": cookie, "Origin": "http://localhost:7777", "X-Amaru-CSRF": csrf}
	target := approvalPath(projectA, env.pending.ID) + "/decision"
	body := decisionBody("approved", env.pending.Evidence.Digest, "looks right", "decide-1")
	first := call(env.h, nil, "POST", target, body, browser)
	check(t, first, 200, "")
	var out map[string]any
	if json.Unmarshal(first.Body.Bytes(), &out) != nil || out["state"] != "approved" {
		t.Fatalf("decision: %s", first.Body)
	}
	decision, _ := out["decision"].(map[string]any)
	approver, _ := decision["approver"].(map[string]any)
	if approver["kind"] != "human" || approver["id"] != "local-administrator" || approver["projectRole"] != "owner" || decision["rationale"] != "looks right" {
		t.Fatalf("decision attribution: %s", first.Body)
	}
	replay := call(env.h, env.auth, "POST", target, body, nil)
	check(t, replay, 200, "")
	if replay.Body.String() != first.Body.String() {
		t.Fatalf("replay differs:\n%s\n%s", first.Body, replay.Body)
	}
	after := env.snapshot(t)
	conflicting := call(env.h, env.auth, "POST", target, decisionBody("rejected", env.pending.Evidence.Digest, "no", "decide-1"), nil)
	check(t, conflicting, 409, "conflict")
	again := call(env.h, env.auth, "POST", target, decisionBody("approved", env.pending.Evidence.Digest, "", "decide-2"), nil)
	check(t, again, 409, "conflict")
	if env.snapshot(t) != after {
		t.Fatal("refused decisions mutated the Approval")
	}
	got := call(env.h, env.auth, "GET", approvalPath(projectA, env.pending.ID), "", nil)
	check(t, got, 200, "")
	if got.Body.String() != first.Body.String() {
		t.Fatal("read after decision differs from the decision response")
	}
}

func TestApprovalDecisionRefusals(t *testing.T) {
	env := approvalSetup(t)
	before := env.snapshot(t)
	target := approvalPath(projectA, env.pending.ID) + "/decision"
	stale := "sha256:" + strings.Repeat("0", 64)
	cases := []struct {
		name, body, status, code string
		headers                  map[string]string
	}{
		{"stale digest", decisionBody("approved", stale, "", "k1"), "409", "conflict", nil},
		{"reject without rationale", decisionBody("rejected", env.pending.Evidence.Digest, "", "k2"), "400", "invalid_request", nil},
		{"rationale over limit", decisionBody("rejected", env.pending.Evidence.Digest, strings.Repeat("x", domain.MaxRationale+1), "k3"), "400", "invalid_request", nil},
		{"unknown field", `{"outcome":"approved","evidenceDigest":"` + env.pending.Evidence.Digest + `","idempotencyKey":"k4","by":"someone"}`, "400", "invalid_request", nil},
		{"missing key", `{"outcome":"approved","evidenceDigest":"` + env.pending.Evidence.Digest + `"}`, "400", "invalid_request", nil},
		{"bad outcome", decisionBody("maybe", env.pending.Evidence.Digest, "", "k5"), "400", "invalid_request", nil},
		{"bad digest", decisionBody("approved", "sha256:abc", "", "k6"), "400", "invalid_request", nil},
		{"two documents", decisionBody("approved", env.pending.Evidence.Digest, "", "k7") + "{}", "400", "invalid_request", nil},
		{"not json", `{`, "400", "invalid_request", nil},
		{"wrong content type", decisionBody("approved", env.pending.Evidence.Digest, "", "k8"), "400", "invalid_request", map[string]string{"Content-Type": "text/plain"}},
		{"oversized", `{"outcome":"approved","rationale":"` + strings.Repeat("x", maxDecisionBody) + `"}`, "400", "invalid_request", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := call(env.h, env.auth, "POST", target, tc.body, tc.headers)
			status := map[string]int{"400": 400, "409": 409}[tc.status]
			check(t, w, status, tc.code)
		})
	}
	if env.snapshot(t) != before {
		t.Fatal("refused decisions mutated the Approval")
	}
}

func TestApprovalBrowserGuards(t *testing.T) {
	env := approvalSetup(t)
	before := env.snapshot(t)
	cookie, csrf := launch(t, env.auth, env.h)
	target := approvalPath(projectA, env.pending.ID) + "/decision"
	body := decisionBody("approved", env.pending.Evidence.Digest, "", "guarded")
	for name, headers := range map[string]map[string]string{
		"no session":   {"Origin": "http://localhost:7777"},
		"no csrf":      {"Cookie": cookie, "Origin": "http://localhost:7777"},
		"wrong csrf":   {"Cookie": cookie, "Origin": "http://localhost:7777", "X-Amaru-CSRF": csrf + "x"},
		"cross origin": {"Cookie": cookie, "Origin": "http://evil.example", "X-Amaru-CSRF": csrf},
		"bad host":     {"Cookie": cookie, "Origin": "http://localhost:7777", "X-Amaru-CSRF": csrf, "Host": "evil.example:7777"},
	} {
		t.Run(name, func(t *testing.T) {
			w := callHost(env.h, "POST", target, body, headers)
			if w.Code < 400 || w.Code >= 500 {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
		})
	}
	if w := callHost(env.h, "GET", approvalPath(projectA, env.pending.ID), "", nil); w.Code != 401 {
		t.Fatalf("unauthenticated read: %d", w.Code)
	}
	if env.snapshot(t) != before {
		t.Fatal("guarded requests mutated the Approval")
	}
}

func TestApprovalSharedDecisionFixtures(t *testing.T) {
	files, err := filepath.Glob("../../../../../packages/contracts/fixtures/api/v1/DecideApproval/*.json")
	if err != nil || len(files) != 5 {
		t.Fatalf("decision fixtures: %d %v", len(files), err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			env := approvalSetup(t)
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			// The fixture digest is a placeholder; substitute the seeded one so a valid body
			// reaches the decision. Invalid bodies stay invalid.
			body := strings.ReplaceAll(string(data), "sha256:"+strings.Repeat("a", 64), env.pending.Evidence.Digest)
			w := call(env.h, env.auth, "POST", approvalPath(projectA, env.pending.ID)+"/decision", body, nil)
			if strings.HasPrefix(filepath.Base(file), "valid") {
				check(t, w, 200, "")
			} else {
				check(t, w, 400, "invalid_request")
			}
		})
	}
}

func TestApprovalRoutesMatchOpenAPI(t *testing.T) {
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
	for _, pattern := range approvalPatterns {
		method, target, _ := strings.Cut(pattern, " ")
		resource := doc.Paths[strings.TrimPrefix(target, "/v1")]
		if _, ok := resource[strings.ToLower(method)]; !ok {
			t.Fatalf("undocumented route: %s", pattern)
		}
	}
	// Every documented Approval operation is served.
	for path, ops := range doc.Paths {
		if !strings.Contains(path, "/approvals") {
			continue
		}
		for method := range ops {
			if method == "parameters" {
				continue
			}
			served := false
			for _, pattern := range approvalPatterns {
				served = served || pattern == strings.ToUpper(method)+" /v1"+path
			}
			if !served {
				t.Fatalf("documented Approval operation not served: %s %s", method, path)
			}
		}
	}
}

func listPath(key domain.ProjectKey, query string) string {
	return "/v1/projects/" + url.PathEscape(string(key)) + "/approvals" + query
}

func TestApprovalListMatchesSharedFixture(t *testing.T) {
	env := approvalSetup(t)
	w := call(env.h, env.auth, "GET", listPath(projectA, ""), "", nil)
	check(t, w, 200, "")
	data, err := os.ReadFile("../../../../../packages/contracts/fixtures/api/v1/ApprovalList/valid-platform-requested.json")
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || json.Unmarshal(data, &want) != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("response differs from the shared fixture:\n%s", w.Body)
	}
	// The default state is requested; naming it explicitly answers the same body.
	explicit := call(env.h, env.auth, "GET", listPath(projectA, "?state=requested"), "", nil)
	check(t, explicit, 200, "")
	if explicit.Body.String() != w.Body.String() {
		t.Fatal("explicit requested state differs from the default")
	}
}

func TestApprovalListIsProjectScopedAndFollowsDecisions(t *testing.T) {
	env := approvalSetup(t)
	ids := func(key domain.ProjectKey, query string) []string {
		t.Helper()
		w := call(env.h, env.auth, "GET", listPath(key, query), "", nil)
		check(t, w, 200, "")
		var out []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out == nil {
			t.Fatalf("list body %q: %v", w.Body, err)
		}
		var got []string
		for _, a := range out {
			if a["projectId"] != string(key) {
				t.Fatalf("foreign Approval listed: %v", a)
			}
			got = append(got, a["id"].(string))
		}
		return got
	}
	if got := ids(projectB, ""); !reflect.DeepEqual(got, []string{"01JD7Q0000000000000000000B"}) {
		t.Fatalf("projectB requested: %v", got)
	}
	decide := call(env.h, env.auth, "POST", approvalPath(projectA, env.pending.ID)+"/decision", decisionBody("approved", env.pending.Evidence.Digest, "", "list-1"), nil)
	check(t, decide, 200, "")
	if got := ids(projectA, ""); got != nil {
		t.Fatalf("decided Approval still requested: %v", got)
	}
	if got := ids(projectA, "?state=approved"); !reflect.DeepEqual(got, []string{env.pending.ID}) {
		t.Fatalf("projectA approved: %v", got)
	}
	if got := ids(projectB, "?state=approved"); got != nil {
		t.Fatalf("projectB approved: %v", got)
	}
	if w := call(env.h, env.auth, "GET", listPath(projectA, "?state=rejected"), "", nil); w.Body.String() != "[]\n" && w.Body.String() != "[]" {
		t.Fatalf("empty list must be an array: %q", w.Body)
	}
}

func TestApprovalListRefusals(t *testing.T) {
	env := approvalSetup(t)
	missing := domain.ProjectKey("ssh://github.com/acme/missing#0123456789abcdef0123456789abcdef01234567")
	for name, tc := range map[string]struct {
		target string
		status int
		code   string
	}{
		"unknown state":   {listPath(projectA, "?state=pending"), 400, "invalid_request"},
		"empty state":     {listPath(projectA, "?state="), 400, "invalid_request"},
		"repeated state":  {listPath(projectA, "?state=requested&state=approved"), 400, "invalid_request"},
		"unknown Project": {listPath(missing, ""), 404, "not_found"},
	} {
		t.Run(name, func(t *testing.T) {
			check(t, call(env.h, env.auth, "GET", tc.target, "", nil), tc.status, tc.code)
		})
	}
	if w := callHost(env.h, "GET", listPath(projectA, ""), "", nil); w.Code != 401 {
		t.Fatalf("unauthenticated list: %d", w.Code)
	}
	if w := callHost(env.h, "GET", listPath(projectA, ""), "", map[string]string{"Host": "evil.example:7777"}); w.Code < 400 || w.Code >= 500 {
		t.Fatalf("foreign Host list: %d", w.Code)
	}
}

// A voided Approval is not representable (no voidedBy); listing it fails the whole response
// rather than dropping or approximating it.
func TestApprovalListRefusesUnrepresentable(t *testing.T) {
	env := approvalSetup(t)
	ctx := context.Background()
	_, steps, err := env.store.Get(ctx, env.pending.Gate.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	a := env.pending
	a.Version = 1
	if err := a.Void(approvalNow, "execution_cancelled"); err != nil {
		t.Fatal(err)
	}
	st := steps[0]
	if err := st.CancelAwaiting(approvalNow); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.Commit(ctx, ports.ExecutionChange{Steps: []domain.StepExecution{st}, Approvals: []domain.Approval{a}}); err != nil {
		t.Fatal(err)
	}
	w := call(env.h, env.auth, "GET", listPath(projectA, "?state=voided"), "", nil)
	check(t, w, 500, "internal")
	if strings.Contains(w.Body.String(), a.ID) {
		t.Fatal("refused list leaked the Approval")
	}
	check(t, call(env.h, env.auth, "GET", listPath(projectB, "?state=voided"), "", nil), 200, "")
}

func TestWireApprovalMapping(t *testing.T) {
	digest := func(c string) string { return "sha256:" + strings.Repeat(c, 64) }
	base := domain.Approval{ID: fixtureApprovalID, ProjectID: string(projectA), State: domain.ApprovalRequested,
		Gate:      domain.ApprovalGate{Kind: domain.GatePolicyRequired, ExecutionID: "exec-a", StepID: "step-1", Capability: "git.push"},
		Requester: domain.Principal{Kind: domain.PrincipalHuman, ID: "owner", Role: domain.RoleOwner}, MinRole: domain.RoleMaintainer,
		Evidence: domain.Evidence{Digest: digest("f"), Items: []domain.EvidenceItem{
			{Kind: "agent", ID: "implementer", Hash: strings.Repeat("4", 64)},
			{Kind: "context", ID: "step-1", Hash: digest("5")},
			{Kind: "definition", ID: "wf", Hash: digest("2")},
			{Kind: "skill", ID: "review@1", Hash: digest("3")},
		}}, RequestedAt: approvalNow}
	pin := &domain.DefinitionPin{WorkflowID: "wf", Revision: strings.Repeat("0", 40), Hash: digest("2")}
	out, err := wireApproval(base, pin)
	if err != nil {
		t.Fatal(err)
	}
	if out.Evidence.Items[0]["hash"] != digest("4") || out.Evidence.Items[2]["commit"] != pin.Revision || out.Evidence.Items[2]["dirty"] != false {
		t.Fatalf("items: %+v", out.Evidence.Items)
	}
	for name, mutate := range map[string]func(*domain.Approval, **domain.DefinitionPin){
		"uncommitted definition": func(_ *domain.Approval, p **domain.DefinitionPin) {
			*p = &domain.DefinitionPin{WorkflowID: "wf", Hash: digest("2")}
		},
		"other definition": func(_ *domain.Approval, p **domain.DefinitionPin) {
			*p = &domain.DefinitionPin{WorkflowID: "other", Revision: "x", Hash: digest("2")}
		},
		"no pin": func(_ *domain.Approval, p **domain.DefinitionPin) { *p = nil },
		"unknown kind": func(a *domain.Approval, _ **domain.DefinitionPin) {
			a.Evidence.Items = []domain.EvidenceItem{{Kind: "mystery", ID: "x"}}
		},
		"voided":                    func(a *domain.Approval, _ **domain.DefinitionPin) { a.State = domain.ApprovalVoided },
		"approved without decision": func(a *domain.Approval, _ **domain.DefinitionPin) { a.State = domain.ApprovalApproved },
		"agent without sponsor": func(a *domain.Approval, _ **domain.DefinitionPin) {
			a.Requester = domain.Principal{Kind: domain.PrincipalAgent, ID: "bot"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			a, p := base, pin
			a.Evidence.Items = append([]domain.EvidenceItem(nil), base.Evidence.Items...)
			mutate(&a, &p)
			if _, err := wireApproval(a, p); !errors.Is(err, errUnrepresentable) {
				t.Fatalf("mapped unrepresentable approval: %v", err)
			}
		})
	}
}
