package httpapi

import (
	"net/http"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/coordinator"
	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
)

// ProductProjects is the authoritative Project store: identity, clone registration and
// settings live in the same Instance state.
type ProductProjects interface {
	ports.ProjectCloneStore
	ports.SettingsStore
}

// ProductConfig names the authoritative Instance dependencies of the served product routes.
// Every dependency must use the same Instance state as Coordinator; ReplayWindow is trusted
// deployment configuration, never client input.
type ProductConfig struct {
	Coordinator  *coordinator.Coordinator
	Projects     ProductProjects
	Inspector    ports.ProjectInspector
	Events       ports.EventStream
	ReplayWindow time.Duration
}

// productRoute pairs a served pattern list with its handler.
type productRoute struct {
	patterns []string
	handler  http.Handler
}

// ProductHandler assembles every served product adapter behind one router bound to auth:
// Projects, settings, Approvals, Executions and Execution Events. Each adapter keeps its own
// guard and authorized application boundary; one ProjectAccess serves the Project-scoped
// routes. A request matching no served route answers the contract's `not_found`. Health and
// authentication routes are served by the Platform HTTP owner and auth, not here.
func ProductHandler(auth *localauth.Auth, config ProductConfig) (http.Handler, error) {
	if auth == nil || config.Coordinator == nil || config.Projects == nil || config.Inspector == nil || config.Events == nil || config.ReplayWindow <= 0 {
		return nil, domain.ErrInvalid
	}
	access, err := coordinator.NewProjectAccess(config.Coordinator, config.Projects, auth.Principal)
	if err != nil {
		return nil, err
	}
	routes, err := productRoutes(auth, access, config)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	for _, route := range routes {
		for _, pattern := range route.patterns {
			mux.Handle(pattern, route.handler)
		}
	}
	// "/" matches every method, so an unknown path and a known path with another method both
	// answer the contract's error shape instead of net/http's plain-text 404/405.
	mux.Handle("/", auth.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reject(w, http.StatusNotFound, "not_found")
	})))
	return mux, nil
}

func productRoutes(auth *localauth.Auth, access *coordinator.ProjectAccess, config ProductConfig) ([]productRoute, error) {
	projects, err := ProjectsHandler(auth, config.Coordinator, config.Projects, config.Inspector)
	if err != nil {
		return nil, err
	}
	settings, err := SettingsHandler(auth, access, config.Projects)
	if err != nil {
		return nil, err
	}
	approvals, err := ApprovalsHandler(auth, access)
	if err != nil {
		return nil, err
	}
	executions, err := ExecutionsHandler(auth, access)
	if err != nil {
		return nil, err
	}
	events, err := ExecutionEventsHandler(auth, access, config.Events, config.ReplayWindow)
	if err != nil {
		return nil, err
	}
	return []productRoute{
		{projectPatterns, projects},
		{settingsPatterns, settings},
		{approvalPatterns, approvals},
		{executionPatterns, executions},
		{executionEventPatterns, events},
	}, nil
}
