package coordinator

import (
	"context"
	"errors"
	"io"

	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/ports"
)

// ArtifactVersions returns only versions produced by Executions in the selected Project.
// index is trusted Instance storage; Artifact IDs alone never establish ownership.
func (a *ProjectAccess) ArtifactVersions(ctx context.Context, project domain.ProjectKey, id string, index ports.ArtifactIndex) ([]domain.ArtifactVersion, error) {
	by, err := a.authorize(ctx, project)
	if err != nil {
		return nil, err
	}
	if index == nil || id == "" || len(id) > domain.MaxArtifactIDLen {
		return nil, domain.ErrInvalid
	}
	versions, err := index.Versions(ctx, id)
	if err != nil {
		return nil, err
	}
	var owned []domain.ArtifactVersion
	seen := make(map[int64]bool, len(versions))
	for _, v := range versions {
		if v.ArtifactID != id || v.Version < 1 || seen[v.Version] {
			return nil, domain.ErrNotFound
		}
		seen[v.Version] = true
		if _, _, err := a.execution(ctx, project, v.ExecutionID); errors.Is(err, domain.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}
		if err := v.Validate(); err != nil {
			return nil, domain.ErrNotFound
		}
		owned = append(owned, v)
	}
	current, err := a.authorize(ctx, project)
	if err != nil {
		return nil, err
	}
	if current.ID != by.ID {
		return nil, domain.ErrNotPermitted
	}
	if len(owned) == 0 {
		return nil, domain.ErrNotFound
	}
	return owned, nil
}

// ArtifactContent opens an exact owned version, never a caller-supplied content hash.
// The caller closes the returned reader. blobs is trusted Instance storage.
func (a *ProjectAccess) ArtifactContent(ctx context.Context, project domain.ProjectKey, id string, version int64, index ports.ArtifactIndex, blobs ports.BlobStore) (io.ReadCloser, error) {
	by, err := a.administrator(ctx)
	if err != nil {
		return nil, err
	}
	versions, err := a.ArtifactVersions(ctx, project, id, index)
	if err != nil {
		return nil, err
	}
	for _, v := range versions {
		if v.Version != version {
			continue
		}
		if blobs == nil {
			return nil, domain.ErrInvalid
		}
		r, err := blobs.Open(ctx, v.Hash)
		if err != nil {
			return nil, err
		}
		current, err := a.authorize(ctx, project)
		if err != nil {
			_ = r.Close()
			return nil, err
		}
		if current.ID != by.ID {
			_ = r.Close()
			return nil, domain.ErrNotPermitted
		}
		return r, nil
	}
	return nil, domain.ErrNotFound
}
