package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
	"github.com/rluders/amaru/services/platform/internal/ports/fakes"
	"go.yaml.in/yaml/v3"
)

func productSetup(t *testing.T) (*localauth.Auth, http.Handler, ProductConfig) {
	t.Helper()
	auth, err := localauth.New(&tcpLoopback)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auth.Close)
	store := fakes.NewExecutionStore()
	projects := fakes.NewProjectStore()
	project := domain.Project{Key: projectA, Name: "a", CreatedAt: approvalNow}
	if _, _, err := projects.RegisterClone(context.Background(), project, domain.ProjectClone{ProjectKey: projectA, Root: "/work/a"}); err != nil {
		t.Fatal(err)
	}
	config := ProductConfig{
		Coordinator: coordinator.New(store, func() time.Time { return approvalNow }),
		Projects:    projects,
		Inspector: projectInspector(func(context.Context, string) (ports.ProjectInspection, error) {
			return ports.ProjectInspection{Key: projectA, Root: "/work/a"}, nil
		}),
		Events:       store,
		ReplayWindow: domain.CompactReplayWindow,
	}
	h, err := ProductHandler(auth, config)
	if err != nil {
		t.Fatal(err)
	}
	return auth, h, config
}

func TestProductHandlerRequiresEveryDependency(t *testing.T) {
	auth, _, full := productSetup(t)
	for name, mutate := range map[string]func(*ProductConfig){
		"coordinator": func(c *ProductConfig) { c.Coordinator = nil },
		"projects":    func(c *ProductConfig) { c.Projects = nil },
		"inspector":   func(c *ProductConfig) { c.Inspector = nil },
		"events":      func(c *ProductConfig) { c.Events = nil },
		"window":      func(c *ProductConfig) { c.ReplayWindow = 0 },
	} {
		config := full
		mutate(&config)
		if h, err := ProductHandler(auth, config); err == nil || h != nil {
			t.Fatalf("%s: accepted a missing dependency", name)
		}
	}
	if _, err := ProductHandler(nil, full); err == nil {
		t.Fatal("accepted a missing Auth")
	}
}

// Every served pattern reaches the router entry registered for it, not the fallback or a
// neighbouring route.
func TestProductHandlerRoutesEveryServedPattern(t *testing.T) {
	_, h, _ := productSetup(t)
	mux := h.(*http.ServeMux)
	for _, route := range [][]string{projectPatterns, settingsPatterns, approvalPatterns, executionPatterns, executionEventPatterns} {
		for _, pattern := range route {
			method, target, _ := strings.Cut(pattern, " ")
			for _, name := range []string{"{projectId}", "{id}", "{approvalId}"} {
				target = strings.ReplaceAll(target, name, "x")
			}
			if _, got := mux.Handler(httptest.NewRequest(method, target, nil)); got != pattern {
				t.Fatalf("%s routed to %q", pattern, got)
			}
		}
	}
}

func TestProductHandlerServesEachFamilyAndRefusesUnknownRoutes(t *testing.T) {
	auth, h, _ := productSetup(t)
	base := "/v1/projects/" + url.PathEscape(string(projectA))
	for _, target := range []string{"/v1/projects", base, base + "/settings", base + "/approvals", base + "/executions"} {
		check(t, call(h, auth, "GET", target, "", nil), 200, "")
	}
	check(t, call(h, auth, "GET", base+"/executions/missing/events", "", nil), 404, "not_found")
	for _, req := range [][2]string{{"GET", "/v1/unknown"}, {"GET", base + "/workflows"}, {"DELETE", base + "/settings"}, {"PUT", "/v1/projects"}} {
		w := call(h, auth, req[0], req[1], "", nil)
		check(t, w, 404, "not_found")
		if w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("%v: %q", req, w.Header().Get("Content-Type"))
		}
	}
	// The fallback keeps the transport guard.
	cookie, _ := launch(t, auth, h)
	if w := callHost(h, "GET", "/v1/unknown", "", map[string]string{"Cookie": cookie, "Host": "evil.example:7777"}); w.Code != 403 {
		t.Fatalf("foreign host on fallback: %d", w.Code)
	}
}

// The product inventory: every documented operation is served by ProductHandler, served by the
// Platform HTTP owner or auth, or pending. A new adapter or contract route must update this list.
func TestProductHandlerInventoryMatchesOpenAPI(t *testing.T) {
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
	served := slices.Concat(projectPatterns, settingsPatterns, approvalPatterns, executionPatterns, executionEventPatterns)
	pending := []string{
		"POST /v1/projects/{projectId}/executions",
		"GET /v1/projects/{projectId}/events",
		"GET /v1/projects/{projectId}/work-packages", "POST /v1/projects/{projectId}/work-packages",
		"GET /v1/projects/{projectId}/work-packages/{id}", "PATCH /v1/projects/{projectId}/work-packages/{id}",
		"DELETE /v1/projects/{projectId}/work-packages/{id}",
		"GET /v1/projects/{projectId}/work-items", "POST /v1/projects/{projectId}/work-items",
		"GET /v1/projects/{projectId}/work-items/{id}", "PATCH /v1/projects/{projectId}/work-items/{id}",
		"DELETE /v1/projects/{projectId}/work-items/{id}",
		"GET /v1/projects/{projectId}/workflows", "POST /v1/projects/{projectId}/workflows",
		"GET /v1/projects/{projectId}/workflows/{id}", "PUT /v1/projects/{projectId}/workflows/{id}",
		"PATCH /v1/projects/{projectId}/workflows/{id}", "DELETE /v1/projects/{projectId}/workflows/{id}",
		"GET /v1/projects/{projectId}/workflows/{id}/source", "POST /v1/projects/{projectId}/workflows/{id}/run",
		"GET /v1/projects/{projectId}/agents", "POST /v1/projects/{projectId}/agents",
		"GET /v1/projects/{projectId}/agents/{id}", "PUT /v1/projects/{projectId}/agents/{id}",
		"PATCH /v1/projects/{projectId}/agents/{id}", "DELETE /v1/projects/{projectId}/agents/{id}",
		"GET /v1/projects/{projectId}/teams", "POST /v1/projects/{projectId}/teams",
		"GET /v1/projects/{projectId}/teams/{id}", "PUT /v1/projects/{projectId}/teams/{id}",
		"PATCH /v1/projects/{projectId}/teams/{id}", "DELETE /v1/projects/{projectId}/teams/{id}",
		"GET /v1/projects/{projectId}/artifacts", "GET /v1/projects/{projectId}/artifacts/{id}",
		"GET /v1/projects/{projectId}/work-packages/{id}/delivery-loop",
		"GET /v1/providers", "GET /v1/capabilities",
	}
	for path, ops := range doc.Paths {
		for method := range ops {
			if method == "parameters" {
				continue
			}
			route := strings.ToUpper(method) + " /v1" + path
			isServed, isPending := slices.Contains(served, route), slices.Contains(pending, route)
			elsewhere := path == "/health" || strings.HasPrefix(path, "/auth/")
			if isServed == (isPending || elsewhere) {
				t.Errorf("product route inventory drifted: %s served=%v pending=%v elsewhere=%v", route, isServed, isPending, elsewhere)
			}
		}
	}
	for _, route := range served {
		method, target, _ := strings.Cut(route, " ")
		if _, ok := doc.Paths[strings.TrimPrefix(target, "/v1")][strings.ToLower(method)]; !ok {
			t.Errorf("undocumented served route: %s", route)
		}
	}
}
