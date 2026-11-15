//go:build pg

package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/rluders/amaru/services/platform/internal/domain"
	"github.com/rluders/amaru/services/platform/internal/store/migrations"
	"github.com/rluders/amaru/services/platform/internal/store/postgres"
)

func TestSettingsPostgresSessionLogoutDuringWrite(t *testing.T) {
	open := projectDatabase(t)
	db := open()
	dir, err := migrations.OpenDir(migrations.Embedded())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := migrations.NewRunner(db, dir, 10*time.Second).Apply(ctx); err != nil {
		t.Fatal("cannot migrate disposable database")
	}
	testSettingsSessionLogoutDuringWrite(t, postgres.NewProjectStore(db))
	if err := db.Close(); err != nil {
		t.Fatal("cannot close settings test pool")
	}
	// A new pool must observe the commit even though the HTTP response was denied.
	reopened := postgres.NewProjectStore(open())
	for _, key := range []domain.ProjectKey{projectA, projectB} {
		got, err := reopened.Settings(ctx, key)
		if err != nil || got.ImportClaudeSkills != (key == projectA) {
			t.Fatal("response denial changed committed Project settings after pool recreation")
		}
	}
}
