package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"github.com/meet-when/meet-when/internal/config"
)

// New creates a new database connection
func New(cfg config.DatabaseConfig) (*sql.DB, error) {
	var db *sql.DB
	var err error

	switch cfg.Driver {
	case "sqlite":
		db, err = sql.Open("sqlite", cfg.ConnectionString())
		if err != nil {
			return nil, fmt.Errorf("failed to open sqlite database: %w", err)
		}
		// SQLite: use single connection to avoid WAL visibility issues
		db.SetMaxOpenConns(1)
		// Enable foreign keys for SQLite
		if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
		}
		// SQLite optimizations for development
		db.Exec("PRAGMA journal_mode = WAL")
		db.Exec("PRAGMA synchronous = NORMAL")
		db.Exec("PRAGMA busy_timeout = 5000")
	case "postgres":
		db, err = sql.Open("postgres", cfg.ConnectionString())
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres database: %w", err)
		}
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// Migrate runs database migrations
func Migrate(db *sql.DB, cfg config.DatabaseConfig) error {
	migrationsPath := cfg.MigrationsPath
	driver := cfg.Driver

	// Use driver-specific migrations if available
	driverMigrationsPath := filepath.Join(migrationsPath, driver)
	if _, err := os.Stat(driverMigrationsPath); err == nil {
		migrationsPath = driverMigrationsPath
	}

	// Create migrations table
	var createTableSQL string
	if driver == "sqlite" {
		createTableSQL = `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				version TEXT PRIMARY KEY,
				applied_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
			)
		`
	} else {
		createTableSQL = `
			CREATE TABLE IF NOT EXISTS schema_migrations (
				version TEXT PRIMARY KEY,
				applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
			)
		`
	}

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get applied migrations
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("failed to scan migration version: %w", err)
		}
		applied[version] = true
	}

	// Get migration files
	files, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrations []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".up.sql") {
			migrations = append(migrations, f.Name())
		}
	}
	sort.Strings(migrations)

	// Apply pending migrations
	for _, migration := range migrations {
		version := strings.TrimSuffix(migration, ".up.sql")
		if applied[version] {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationsPath, migration))
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", migration, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		// For SQLite, execute statements one by one (SQLite doesn't support multiple statements in one Exec)
		if driver == "sqlite" {
			statements := splitSQLStatements(string(content))
			for _, stmt := range statements {
				stmt = strings.TrimSpace(stmt)
				if stmt == "" {
					continue
				}
				if _, err := tx.Exec(stmt); err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to apply migration %s: %w\nStatement: %s", migration, err, stmt)
				}
			}
		} else {
			if _, err := tx.Exec(string(content)); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to apply migration %s: %w", migration, err)
			}
		}

		// Record migration - use ? placeholder for SQLite, $1 for Postgres
		var insertSQL string
		if driver == "sqlite" {
			insertSQL = "INSERT INTO schema_migrations (version) VALUES (?)"
		} else {
			insertSQL = "INSERT INTO schema_migrations (version) VALUES ($1)"
		}

		if _, err := tx.Exec(insertSQL, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", migration, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", migration, err)
		}

		fmt.Printf("Applied migration: %s\n", version)
	}

	return nil
}

// splitSQLStatements splits SQL content into individual statements
func splitSQLStatements(content string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	stringChar := rune(0)

	for i, ch := range content {
		if !inString && (ch == '\'' || ch == '"') {
			inString = true
			stringChar = ch
		} else if inString && ch == stringChar {
			// Check for escaped quote
			if i+1 < len(content) && rune(content[i+1]) == stringChar {
				current.WriteRune(ch)
				continue
			}
			inString = false
		}

		if ch == ';' && !inString {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
		} else {
			current.WriteRune(ch)
		}
	}

	// Don't forget the last statement if no trailing semicolon
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}
