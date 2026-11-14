package coordinator

import (
	"context"

	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
)

// Settings rechecks current identity and attachment before reading preferences.
// store is a trusted composition dependency, never supplied by a client.
func (a *ProjectAccess) Settings(ctx context.Context, project domain.ProjectKey, store ports.SettingsStore) (domain.ProjectSettings, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return domain.ProjectSettings{}, err
	}
	if store == nil {
		return domain.ProjectSettings{}, domain.ErrInvalid
	}
	settings, err := store.Settings(ctx, project)
	if err != nil {
		return domain.ProjectSettings{}, err
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return domain.ProjectSettings{}, err
	}
	if current.ID != by.ID {
		return domain.ProjectSettings{}, domain.ErrNotPermitted
	}
	return settings, nil
}

// UpdateSettings is an unversioned partial update, not a replayable execution command.
func (a *ProjectAccess) UpdateSettings(ctx context.Context, project domain.ProjectKey, patch domain.UpdateSettings, store ports.SettingsStore) (domain.ProjectSettings, error) {
	if _, err := a.authorize(ctx, project); err != nil {
		return domain.ProjectSettings{}, err
	}
	if err := patch.Validate(); err != nil {
		return domain.ProjectSettings{}, err
	}
	if store == nil {
		return domain.ProjectSettings{}, domain.ErrInvalid
	}
	return store.UpdateSettings(ctx, project, patch)
}
