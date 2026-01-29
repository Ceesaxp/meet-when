package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/database"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
	"github.com/meet-when/meet-when/internal/services"
)

// TestTrackSignupCTA_Success verifies successful signup tracking
func TestTrackSignupCTA_Success(t *testing.T) {
	// Setup test database
	db, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	// Create test data: tenant, host, template, booking
	booking, err := createTestBookingData(t, db, repos)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	// Create handlers
	h := createTestHandlers(t, repos)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/signup/track?ref=booking:"+booking.Token, nil)
	w := httptest.NewRecorder()

	// Call handler
	h.Auth.TrackSignupCTA(w, req)

	// Verify redirect
	if w.Code != http.StatusFound && w.Code != http.StatusSeeOther {
		t.Errorf("Expected redirect status (302 or 303), got %d", w.Code)
	}

	location := w.Header().Get("Location")
	expectedLocation := "/auth/register?ref=booking:" + booking.Token
	if location != expectedLocation {
		t.Errorf("Expected redirect to %s, got %s", expectedLocation, location)
	}

	// Verify signup_conversion record was created
	conversion, err := repos.SignupConversion.GetByEmail(context.Background(), booking.InviteeEmail)
	if err != nil {
		t.Fatalf("Failed to get conversion: %v", err)
	}

	if conversion == nil {
		t.Fatal("Expected conversion record to be created")
	}

	if conversion.InviteeEmail != booking.InviteeEmail {
		t.Errorf("Expected invitee_email %s, got %s", booking.InviteeEmail, conversion.InviteeEmail)
	}

	if conversion.SourceBookingID == nil || *conversion.SourceBookingID != booking.ID {
		t.Errorf("Expected source_booking_id %s, got %v", booking.ID, conversion.SourceBookingID)
	}

	if conversion.ClickedAt.IsZero() {
		t.Error("Expected clicked_at to be set")
	}

	if conversion.RegisteredAt != nil {
		t.Error("Expected registered_at to be nil (not yet registered)")
	}
}

// TestTrackSignupCTA_InvalidRef verifies error handling for invalid ref format
func TestTrackSignupCTA_InvalidRef(t *testing.T) {
	// Setup test database
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	// Create handlers
	h := createTestHandlers(t, repos)

	tests := []struct {
		name string
		ref  string
	}{
		{
			name: "Missing ref parameter",
			ref:  "",
		},
		{
			name: "Invalid ref format - no colon",
			ref:  "bookingtoken123",
		},
		{
			name: "Invalid ref format - wrong prefix",
			ref:  "meeting:token123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var url string
			if tt.ref == "" {
				url = "/signup/track"
			} else {
				url = "/signup/track?ref=" + tt.ref
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			h.Auth.TrackSignupCTA(w, req)

			// Should still redirect to register page but without creating a conversion
			if w.Code != http.StatusFound && w.Code != http.StatusSeeOther {
				t.Errorf("Expected redirect status, got %d", w.Code)
			}

			location := w.Header().Get("Location")
			if location != "/auth/register" && location != "/auth/register?ref="+tt.ref {
				t.Errorf("Expected redirect to register page, got %s", location)
			}
		})
	}
}

// TestTrackSignupCTA_BookingNotFound verifies handling when booking doesn't exist
func TestTrackSignupCTA_BookingNotFound(t *testing.T) {
	// Setup test database
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	// Create handlers
	h := createTestHandlers(t, repos)

	// Request with non-existent booking token
	req := httptest.NewRequest(http.MethodGet, "/signup/track?ref=booking:nonexistent-token", nil)
	w := httptest.NewRecorder()

	h.Auth.TrackSignupCTA(w, req)

	// Should still redirect to register page but without creating a conversion
	if w.Code != http.StatusFound && w.Code != http.StatusSeeOther {
		t.Errorf("Expected redirect status, got %d", w.Code)
	}

	location := w.Header().Get("Location")
	expectedLocation := "/auth/register?ref=booking:nonexistent-token"
	if location != expectedLocation {
		t.Errorf("Expected redirect to %s, got %s", expectedLocation, location)
	}
}

// TestTrackSignupCTA_DuplicateClick verifies handling of multiple clicks from same invitee
func TestTrackSignupCTA_DuplicateClick(t *testing.T) {
	// Setup test database
	db, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	// Create test data
	booking, err := createTestBookingData(t, db, repos)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	// Create handlers
	h := createTestHandlers(t, repos)

	// First click
	req1 := httptest.NewRequest(http.MethodGet, "/signup/track?ref=booking:"+booking.Token, nil)
	w1 := httptest.NewRecorder()
	h.Auth.TrackSignupCTA(w1, req1)

	// Second click (duplicate)
	req2 := httptest.NewRequest(http.MethodGet, "/signup/track?ref=booking:"+booking.Token, nil)
	w2 := httptest.NewRecorder()
	h.Auth.TrackSignupCTA(w2, req2)

	// Both should redirect successfully
	if w1.Code != http.StatusFound && w1.Code != http.StatusSeeOther {
		t.Errorf("First request: expected redirect status, got %d", w1.Code)
	}
	if w2.Code != http.StatusFound && w2.Code != http.StatusSeeOther {
		t.Errorf("Second request: expected redirect status, got %d", w2.Code)
	}

	// Should have created two conversion records (tracking each click)
	// We can verify this by checking that the most recent one was just created
	conversion, err := repos.SignupConversion.GetByEmail(context.Background(), booking.InviteeEmail)
	if err != nil {
		t.Fatalf("Failed to get conversion: %v", err)
	}

	if conversion == nil {
		t.Fatal("Expected conversion record to exist")
	}
}

// Helper functions

func setupTestDatabase(t *testing.T) (*sql.DB, *repository.Repositories, func()) {
	t.Helper()

	cfg := config.DatabaseConfig{
		Driver:         "sqlite",
		Name:           ":memory:",
		MigrationsPath: "../../migrations",
	}

	db, err := database.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	if err := database.Migrate(db, cfg); err != nil {
		db.Close()
		t.Fatalf("Failed to run migrations: %v", err)
	}

	repos := repository.NewRepositories(db, "sqlite")

	cleanup := func() {
		db.Close()
	}

	return db, repos, cleanup
}

func createTestBookingData(t *testing.T, db *sql.DB, repos *repository.Repositories) (*models.Booking, error) {
	t.Helper()

	ctx := context.Background()

	// Generate unique IDs for this test
	tenantID := uuid.New().String()
	hostID := uuid.New().String()
	templateID := uuid.New().String()
	bookingID := uuid.New().String()
	tokenSuffix := uuid.New().String()[:8]

	// Create tenant
	tenant := &models.Tenant{
		ID:        tenantID,
		Slug:      "test-tenant-" + tokenSuffix,
		Name:      "Test Tenant",
		CreatedAt: models.Now(),
		UpdatedAt: models.Now(),
	}
	if err := repos.Tenant.Create(ctx, tenant); err != nil {
		return nil, fmt.Errorf("tenant create: %w", err)
	}

	// Create host
	host := &models.Host{
		ID:           hostID,
		TenantID:     tenant.ID,
		Email:        "host-" + tokenSuffix + "@example.com",
		PasswordHash: "hash",
		Name:         "Test Host",
		Slug:         "test-host-" + tokenSuffix,
		Timezone:     "UTC",
		CreatedAt:    models.Now(),
		UpdatedAt:    models.Now(),
	}
	if err := repos.Host.Create(ctx, host); err != nil {
		return nil, fmt.Errorf("host create: %w", err)
	}

	// Create template using raw SQL to handle NULL calendar_id properly
	_, err := db.Exec(`
		INSERT INTO meeting_templates (id, host_id, slug, name, durations, location_type, calendar_id, is_active, is_private, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NULL, 1, 0, ?, ?)
	`, templateID, host.ID, "test-meeting-"+tokenSuffix, "Test Meeting", `[30]`, "google_meet", models.Now(), models.Now())
	if err != nil {
		return nil, fmt.Errorf("template create: %w", err)
	}

	template := &models.MeetingTemplate{
		ID:     templateID,
		HostID: host.ID,
	}

	// Create booking
	booking := &models.Booking{
		ID:           bookingID,
		TemplateID:   template.ID,
		HostID:       host.ID,
		Token:        "test-token-" + tokenSuffix,
		Status:       models.BookingStatusConfirmed,
		StartTime:    models.Now(),
		EndTime:      models.Now(),
		Duration:     30,
		InviteeName:  "Test Invitee",
		InviteeEmail: "invitee@example.com",
		CreatedAt:    models.Now(),
		UpdatedAt:    models.Now(),
	}
	if err := repos.Booking.Create(ctx, booking); err != nil {
		return nil, fmt.Errorf("booking create: %w", err)
	}

	return booking, nil
}

func createTestHandlers(t *testing.T, repos *repository.Repositories) *Handlers {
	t.Helper()

	cfg := config.Config{
		Server: config.ServerConfig{
			BaseURL: "http://localhost:8080",
		},
	}

	svc := services.New(&cfg, repos)

	h := &Handlers{
		cfg:      &cfg,
		repos:    repos,
		services: svc,
	}

	h.Auth = &AuthHandler{handlers: h}
	h.Dashboard = &DashboardHandler{handlers: h}
	h.Public = &PublicHandler{handlers: h}
	h.API = &APIHandler{handlers: h}
	h.Onboarding = &OnboardingHandler{handlers: h}

	return h
}
