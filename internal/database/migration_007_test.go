package database

import (
	"database/sql"
	"testing"

	"github.com/meet-when/meet-when/internal/config"
)

// TestMigration007_SignupConversions verifies that the signup_conversions migration applies successfully
func TestMigration007_SignupConversions(t *testing.T) {
	tests := []struct {
		name   string
		driver string
	}{
		{
			name:   "PostgreSQL",
			driver: "postgres",
		},
		{
			name:   "SQLite",
			driver: "sqlite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip postgres test if not available
			if tt.driver == "postgres" && !isPostgresAvailable() {
				t.Skip("PostgreSQL not available")
			}

			// Create test database
			db, cleanup := setupTestDB(t, tt.driver)
			defer cleanup()

			// Verify signup_conversions table exists
			var tableName string
			var query string
			if tt.driver == "sqlite" {
				query = "SELECT name FROM sqlite_master WHERE type='table' AND name='signup_conversions'"
			} else {
				query = "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name='signup_conversions'"
			}

			err := db.QueryRow(query).Scan(&tableName)
			if err != nil {
				t.Fatalf("signup_conversions table should exist after migration: %v", err)
			}

			if tableName != "signup_conversions" {
				t.Fatalf("expected table name 'signup_conversions', got '%s'", tableName)
			}

			// Verify table structure by attempting to insert a test record
			testInsert := `
				INSERT INTO signup_conversions (
					id, source_booking_id, invitee_email, clicked_at, tenant_id, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?)
			`
			if tt.driver == "postgres" {
				testInsert = `
					INSERT INTO signup_conversions (
						id, source_booking_id, invitee_email, clicked_at, tenant_id, created_at, updated_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7)
				`
			}

			// First create a tenant and booking for foreign key constraints
			var tenantID, bookingID string
			if tt.driver == "sqlite" {
				tenantID = "test-tenant-001"
				bookingID = "test-booking-001"
				_, err = db.Exec(`INSERT INTO tenants (id, slug, name) VALUES (?, ?, ?)`, tenantID, "test", "Test Tenant")
			} else {
				err = db.QueryRow(`INSERT INTO tenants (slug, name) VALUES ($1, $2) RETURNING id`, "test", "Test Tenant").Scan(&tenantID)
			}
			if err != nil {
				t.Fatalf("failed to create test tenant: %v", err)
			}

			// Create a test host for the booking
			var hostID string
			if tt.driver == "sqlite" {
				hostID = "test-host-001"
				_, err = db.Exec(`INSERT INTO hosts (id, tenant_id, email, password_hash, name, slug) VALUES (?, ?, ?, ?, ?, ?)`,
					hostID, tenantID, "host@example.com", "hash", "Test Host", "host")
			} else {
				err = db.QueryRow(`INSERT INTO hosts (tenant_id, email, password_hash, name, slug) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
					tenantID, "host@example.com", "hash", "Test Host", "host").Scan(&hostID)
			}
			if err != nil {
				t.Fatalf("failed to create test host: %v", err)
			}

			// Create a test template for the booking
			var templateID string
			if tt.driver == "sqlite" {
				templateID = "test-template-001"
				_, err = db.Exec(`INSERT INTO meeting_templates (id, host_id, slug, name) VALUES (?, ?, ?, ?)`,
					templateID, hostID, "test", "Test Template")
			} else {
				err = db.QueryRow(`INSERT INTO meeting_templates (host_id, slug, name) VALUES ($1, $2, $3) RETURNING id`,
					hostID, "test", "Test Template").Scan(&templateID)
			}
			if err != nil {
				t.Fatalf("failed to create test template: %v", err)
			}

			// Create a test booking
			if tt.driver == "sqlite" {
				bookingID = "test-booking-001"
				_, err = db.Exec(`INSERT INTO bookings (id, template_id, host_id, token, status, start_time, end_time, duration, invitee_name, invitee_email)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					bookingID, templateID, hostID, "test-token", "confirmed", "2026-02-01T10:00:00Z", "2026-02-01T11:00:00Z", 60, "Invitee", "invitee@example.com")
			} else {
				err = db.QueryRow(`INSERT INTO bookings (template_id, host_id, token, status, start_time, end_time, duration, invitee_name, invitee_email)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
					templateID, hostID, "test-token", "confirmed", "2026-02-01T10:00:00Z", "2026-02-01T11:00:00Z", 60, "Invitee", "invitee@example.com").Scan(&bookingID)
			}
			if err != nil {
				t.Fatalf("failed to create test booking: %v", err)
			}

			// Now test inserting into signup_conversions
			var conversionID string
			if tt.driver == "sqlite" {
				conversionID = "test-conversion-001"
				_, err = db.Exec(testInsert,
					conversionID, bookingID, "test@example.com", "2026-01-29T10:00:00Z", tenantID, "2026-01-29T10:00:00Z", "2026-01-29T10:00:00Z")
			} else {
				err = db.QueryRow(`
					INSERT INTO signup_conversions (
						source_booking_id, invitee_email, clicked_at, tenant_id
					) VALUES ($1, $2, NOW(), $3) RETURNING id`,
					bookingID, "test@example.com", tenantID).Scan(&conversionID)
			}
			if err != nil {
				t.Fatalf("failed to insert test record into signup_conversions: %v", err)
			}

			// Verify we can query the record
			var email string
			if tt.driver == "sqlite" {
				err = db.QueryRow("SELECT invitee_email FROM signup_conversions WHERE id = ?", conversionID).Scan(&email)
			} else {
				err = db.QueryRow("SELECT invitee_email FROM signup_conversions WHERE id = $1", conversionID).Scan(&email)
			}
			if err != nil {
				t.Fatalf("failed to query inserted record: %v", err)
			}

			if email != "test@example.com" {
				t.Errorf("expected email 'test@example.com', got '%s'", email)
			}

			// Verify nullable registered_at column
			var registeredAt sql.NullString
			if tt.driver == "sqlite" {
				err = db.QueryRow("SELECT registered_at FROM signup_conversions WHERE id = ?", conversionID).Scan(&registeredAt)
			} else {
				err = db.QueryRow("SELECT registered_at FROM signup_conversions WHERE id = $1", conversionID).Scan(&registeredAt)
			}
			if err != nil {
				t.Fatalf("failed to query registered_at: %v", err)
			}

			if registeredAt.Valid {
				t.Errorf("expected registered_at to be NULL, got '%s'", registeredAt.String)
			}

			// Verify indexes exist by checking query plan (basic verification)
			// This is a simple check - in real usage, the database would use these indexes
			if tt.driver == "postgres" {
				var indexName string
				err = db.QueryRow(`
					SELECT indexname FROM pg_indexes
					WHERE tablename = 'signup_conversions' AND indexname = 'idx_signup_conversions_invitee_email'
				`).Scan(&indexName)
				if err != nil {
					t.Fatalf("index idx_signup_conversions_invitee_email should exist: %v", err)
				}
			}
		})
	}
}

// setupTestDB creates a test database with all migrations applied
func setupTestDB(t *testing.T, driver string) (*sql.DB, func()) {
	t.Helper()

	var cfg config.DatabaseConfig
	if driver == "sqlite" {
		cfg = config.DatabaseConfig{
			Driver:         "sqlite",
			Name:           ":memory:",
			MigrationsPath: "../../migrations",
		}
	} else {
		cfg = config.DatabaseConfig{
			Driver:         "postgres",
			Host:           "localhost",
			Port:           5432,
			User:           "postgres",
			Password:       "postgres",
			Name:           "meetwhen_test",
			MigrationsPath: "../../migrations",
		}
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Run migrations
	if err := Migrate(db, cfg); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

// isPostgresAvailable checks if PostgreSQL is available for testing
func isPostgresAvailable() bool {
	cfg := config.DatabaseConfig{
		Driver:   "postgres",
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		Name:     "postgres",
	}

	db, err := sql.Open("postgres", cfg.ConnectionString())
	if err != nil {
		return false
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return false
	}

	return true
}
