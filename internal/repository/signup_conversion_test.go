package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/database"
	"github.com/meet-when/meet-when/internal/models"
)

func TestSignupConversionRepository_Create(t *testing.T) {
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

			db, cleanup := setupTestDB(t, tt.driver)
			defer cleanup()

			repo := &SignupConversionRepository{db: db, driver: tt.driver}

			// Create test tenant and booking for foreign key constraints
			tenant, _, booking := createTestBooking(t, db, tt.driver)

			// Create a signup conversion
			conversion := &models.SignupConversion{
				ID:              generateID(tt.driver),
				SourceBookingID: &booking.ID,
				InviteeEmail:    "test@example.com",
				ClickedAt:       models.Now(),
				RegisteredAt:    nil,
				TenantID:        tenant.ID,
				CreatedAt:       models.Now(),
				UpdatedAt:       models.Now(),
			}

			err := repo.Create(context.Background(), conversion)
			if err != nil {
				t.Fatalf("Create failed: %v", err)
			}

			// Verify it was created by fetching it
			var count int
			var query string
			if tt.driver == "sqlite" {
				query = "SELECT COUNT(*) FROM signup_conversions WHERE id = ?"
			} else {
				query = "SELECT COUNT(*) FROM signup_conversions WHERE id = $1"
			}
			err = db.QueryRow(query, conversion.ID).Scan(&count)
			if err != nil {
				t.Fatalf("Failed to verify creation: %v", err)
			}

			if count != 1 {
				t.Errorf("expected 1 record, got %d", count)
			}
		})
	}
}

func TestSignupConversionRepository_MarkRegistered(t *testing.T) {
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
			if tt.driver == "postgres" && !isPostgresAvailable() {
				t.Skip("PostgreSQL not available")
			}

			db, cleanup := setupTestDB(t, tt.driver)
			defer cleanup()

			repo := &SignupConversionRepository{db: db, driver: tt.driver}

			// Create test data
			tenant, _, booking := createTestBooking(t, db, tt.driver)

			// Create a conversion with no registration
			conversion := &models.SignupConversion{
				ID:              generateID(tt.driver),
				SourceBookingID: &booking.ID,
				InviteeEmail:    "test@example.com",
				ClickedAt:       models.Now(),
				RegisteredAt:    nil,
				TenantID:        tenant.ID,
				CreatedAt:       models.Now(),
				UpdatedAt:       models.Now(),
			}

			err := repo.Create(context.Background(), conversion)
			if err != nil {
				t.Fatalf("Create failed: %v", err)
			}

			// Mark as registered
			beforeMark := time.Now().UTC().Add(-1 * time.Second) // Add buffer for test reliability
			err = repo.MarkRegistered(context.Background(), conversion.InviteeEmail)
			if err != nil {
				t.Fatalf("MarkRegistered failed: %v", err)
			}
			afterMark := time.Now().UTC().Add(1 * time.Second) // Add buffer for test reliability

			// Verify registered_at was set
			var registeredAt models.SQLiteTime
			var query string
			if tt.driver == "sqlite" {
				query = "SELECT registered_at FROM signup_conversions WHERE id = ?"
			} else {
				query = "SELECT registered_at FROM signup_conversions WHERE id = $1"
			}
			err = db.QueryRow(query, conversion.ID).Scan(&registeredAt)
			if err != nil {
				t.Fatalf("Failed to query registered_at: %v", err)
			}

			// Verify registered_at is not nil (was set)
			if registeredAt.IsZero() {
				t.Error("registered_at should be set, got zero time")
			}

			// Verify the timestamp is reasonable (between before and after the call)
			// Use UTC for consistent comparison
			if registeredAt.UTC().Before(beforeMark) || registeredAt.UTC().After(afterMark) {
				t.Errorf("registered_at %v should be between %v and %v",
					registeredAt.UTC(), beforeMark, afterMark)
			}
		})
	}
}

func TestSignupConversionRepository_GetByEmail(t *testing.T) {
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
			if tt.driver == "postgres" && !isPostgresAvailable() {
				t.Skip("PostgreSQL not available")
			}

			db, cleanup := setupTestDB(t, tt.driver)
			defer cleanup()

			repo := &SignupConversionRepository{db: db, driver: tt.driver}

			// Create test data
			tenant, _, booking := createTestBooking(t, db, tt.driver)

			// Create a conversion
			conversion := &models.SignupConversion{
				ID:              generateID(tt.driver),
				SourceBookingID: &booking.ID,
				InviteeEmail:    "test@example.com",
				ClickedAt:       models.Now(),
				RegisteredAt:    nil,
				TenantID:        tenant.ID,
				CreatedAt:       models.Now(),
				UpdatedAt:       models.Now(),
			}

			err := repo.Create(context.Background(), conversion)
			if err != nil {
				t.Fatalf("Create failed: %v", err)
			}

			// Test: Get by email - should find it
			result, err := repo.GetByEmail(context.Background(), "test@example.com")
			if err != nil {
				t.Fatalf("GetByEmail failed: %v", err)
			}

			if result == nil {
				t.Fatal("expected to find conversion, got nil")
			}

			if result.InviteeEmail != conversion.InviteeEmail {
				t.Errorf("expected email %s, got %s", conversion.InviteeEmail, result.InviteeEmail)
			}

			if result.TenantID != conversion.TenantID {
				t.Errorf("expected tenant_id %s, got %s", conversion.TenantID, result.TenantID)
			}

			// Test: Get by non-existent email - should return nil
			result, err = repo.GetByEmail(context.Background(), "nonexistent@example.com")
			if err != nil {
				t.Fatalf("GetByEmail failed for non-existent email: %v", err)
			}

			if result != nil {
				t.Errorf("expected nil for non-existent email, got %v", result)
			}
		})
	}
}

func TestSignupConversionRepository_GetByEmail_MostRecent(t *testing.T) {
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
			if tt.driver == "postgres" && !isPostgresAvailable() {
				t.Skip("PostgreSQL not available")
			}

			db, cleanup := setupTestDB(t, tt.driver)
			defer cleanup()

			repo := &SignupConversionRepository{db: db, driver: tt.driver}

			// Create test data
			tenant, _, booking := createTestBooking(t, db, tt.driver)

			// Create multiple conversions for the same email
			email := "test@example.com"

			// First conversion (older)
			conversion1 := &models.SignupConversion{
				ID:              generateID(tt.driver),
				SourceBookingID: &booking.ID,
				InviteeEmail:    email,
				ClickedAt:       models.NewSQLiteTime(time.Now().Add(-2 * time.Hour)),
				RegisteredAt:    nil,
				TenantID:        tenant.ID,
				CreatedAt:       models.NewSQLiteTime(time.Now().Add(-2 * time.Hour)),
				UpdatedAt:       models.NewSQLiteTime(time.Now().Add(-2 * time.Hour)),
			}

			err := repo.Create(context.Background(), conversion1)
			if err != nil {
				t.Fatalf("Create first conversion failed: %v", err)
			}

			// Second conversion (newer)
			conversion2 := &models.SignupConversion{
				ID:              generateID(tt.driver),
				SourceBookingID: &booking.ID,
				InviteeEmail:    email,
				ClickedAt:       models.Now(),
				RegisteredAt:    nil,
				TenantID:        tenant.ID,
				CreatedAt:       models.Now(),
				UpdatedAt:       models.Now(),
			}

			err = repo.Create(context.Background(), conversion2)
			if err != nil {
				t.Fatalf("Create second conversion failed: %v", err)
			}

			// Get by email - should return the most recent one
			result, err := repo.GetByEmail(context.Background(), email)
			if err != nil {
				t.Fatalf("GetByEmail failed: %v", err)
			}

			if result == nil {
				t.Fatal("expected to find conversion, got nil")
			}

			// Verify it's the most recent one (conversion2)
			if result.ID != conversion2.ID {
				t.Errorf("expected most recent conversion ID %s, got %s", conversion2.ID, result.ID)
			}
		})
	}
}

// Helper functions

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

	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Run migrations
	if err := database.Migrate(db, cfg); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

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

func createTestBooking(t *testing.T, db *sql.DB, driver string) (*models.Tenant, *models.Host, *models.Booking) {
	t.Helper()

	// Create tenant
	tenant := &models.Tenant{
		ID:        generateID(driver),
		Slug:      "test-tenant",
		Name:      "Test Tenant",
		CreatedAt: models.Now(),
		UpdatedAt: models.Now(),
	}

	var err error
	if driver == "sqlite" {
		_, err = db.Exec(`INSERT INTO tenants (id, slug, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			tenant.ID, tenant.Slug, tenant.Name, tenant.CreatedAt, tenant.UpdatedAt)
	} else {
		err = db.QueryRow(`INSERT INTO tenants (slug, name, created_at, updated_at) VALUES ($1, $2, $3, $4) RETURNING id`,
			tenant.Slug, tenant.Name, tenant.CreatedAt, tenant.UpdatedAt).Scan(&tenant.ID)
	}
	if err != nil {
		t.Fatalf("failed to create test tenant: %v", err)
	}

	// Create host
	host := &models.Host{
		ID:           generateID(driver),
		TenantID:     tenant.ID,
		Email:        "host@example.com",
		PasswordHash: "hash",
		Name:         "Test Host",
		Slug:         "host",
		Timezone:     "UTC",
		CreatedAt:    models.Now(),
		UpdatedAt:    models.Now(),
	}

	if driver == "sqlite" {
		_, err = db.Exec(`INSERT INTO hosts (id, tenant_id, email, password_hash, name, slug, timezone, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			host.ID, host.TenantID, host.Email, host.PasswordHash, host.Name, host.Slug, host.Timezone, host.CreatedAt, host.UpdatedAt)
	} else {
		err = db.QueryRow(`INSERT INTO hosts (tenant_id, email, password_hash, name, slug, timezone, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			host.TenantID, host.Email, host.PasswordHash, host.Name, host.Slug, host.Timezone, host.CreatedAt, host.UpdatedAt).Scan(&host.ID)
	}
	if err != nil {
		t.Fatalf("failed to create test host: %v", err)
	}

	// Create template
	template := &models.MeetingTemplate{
		ID:        generateID(driver),
		HostID:    host.ID,
		Slug:      "test-template",
		Name:      "Test Template",
		CreatedAt: models.Now(),
		UpdatedAt: models.Now(),
	}

	if driver == "sqlite" {
		_, err = db.Exec(`INSERT INTO meeting_templates (id, host_id, slug, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			template.ID, template.HostID, template.Slug, template.Name, template.CreatedAt, template.UpdatedAt)
	} else {
		err = db.QueryRow(`INSERT INTO meeting_templates (host_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			template.HostID, template.Slug, template.Name, template.CreatedAt, template.UpdatedAt).Scan(&template.ID)
	}
	if err != nil {
		t.Fatalf("failed to create test template: %v", err)
	}

	// Create booking
	booking := &models.Booking{
		ID:          generateID(driver),
		TemplateID:  template.ID,
		HostID:      host.ID,
		Token:       "test-token-" + generateID(driver),
		Status:      models.BookingStatusConfirmed,
		StartTime:   models.NewSQLiteTime(time.Now().Add(24 * time.Hour)),
		EndTime:     models.NewSQLiteTime(time.Now().Add(25 * time.Hour)),
		Duration:    60,
		InviteeName: "Invitee",
		InviteeEmail: "invitee@example.com",
		CreatedAt:   models.Now(),
		UpdatedAt:   models.Now(),
	}

	if driver == "sqlite" {
		_, err = db.Exec(`INSERT INTO bookings (id, template_id, host_id, token, status, start_time, end_time, duration, invitee_name, invitee_email, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			booking.ID, booking.TemplateID, booking.HostID, booking.Token, booking.Status,
			booking.StartTime, booking.EndTime, booking.Duration, booking.InviteeName, booking.InviteeEmail,
			booking.CreatedAt, booking.UpdatedAt)
	} else {
		err = db.QueryRow(`INSERT INTO bookings (template_id, host_id, token, status, start_time, end_time, duration, invitee_name, invitee_email, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			booking.TemplateID, booking.HostID, booking.Token, booking.Status,
			booking.StartTime, booking.EndTime, booking.Duration, booking.InviteeName, booking.InviteeEmail,
			booking.CreatedAt, booking.UpdatedAt).Scan(&booking.ID)
	}
	if err != nil {
		t.Fatalf("failed to create test booking: %v", err)
	}

	return tenant, host, booking
}

func generateID(driver string) string {
	if driver == "sqlite" {
		return "test-" + time.Now().Format("20060102150405.000000")
	}
	// For postgres, let the DB generate UUID
	return ""
}
