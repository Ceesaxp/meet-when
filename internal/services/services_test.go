package services

import (
	"database/sql"
	"os"
	"testing"

	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/database"
	"github.com/meet-when/meet-when/internal/repository"
)

// setupTestRepos opens an in-memory sqlite DB, applies all migrations, and
// returns wired repositories. Used by service-level tests that need real
// schema persistence (e.g. tracking-row writes in the syncer).
func setupTestRepos(t *testing.T) (*sql.DB, *repository.Repositories, func()) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	cfg := config.DatabaseConfig{
		Driver:         "sqlite",
		Name:           ":memory:",
		MigrationsPath: cwd + "/../../migrations",
	}

	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := database.Migrate(db, cfg); err != nil {
		_ = db.Close()
		t.Fatalf("migrate: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
	}

	return db, repository.NewRepositories(db, "sqlite"), cleanup
}
