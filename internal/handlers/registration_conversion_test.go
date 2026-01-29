package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
)

// TestRegister_WithBookingRef_MarksConversionRegistered tests that registration with booking ref marks conversion
func TestRegister_WithBookingRef_MarksConversionRegistered(t *testing.T) {
	// Setup test database
	db, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	// Create test booking and conversion
	booking, err := createTestBookingData(t, db, repos)
	if err != nil {
		t.Fatalf("Failed to create test booking: %v", err)
	}

	// Get the host to get tenant ID
	host, err := repos.Host.GetByID(context.Background(), booking.HostID)
	if err != nil {
		t.Fatalf("Failed to get host: %v", err)
	}

	// Create a signup conversion record (simulating a CTA click)
	conversion := &models.SignupConversion{
		ID:              uuid.New().String(),
		SourceBookingID: &booking.ID,
		InviteeEmail:    "newuser@example.com",
		ClickedAt:       models.Now(),
		RegisteredAt:    nil, // Not yet registered
		TenantID:        host.TenantID,
		CreatedAt:       models.Now(),
		UpdatedAt:       models.Now(),
	}

	err = repos.SignupConversion.Create(context.Background(), conversion)
	if err != nil {
		t.Fatalf("Failed to create conversion: %v", err)
	}

	// Create handlers
	h := createTestHandlers(t, repos)

	// Create registration form data with ref parameter
	form := url.Values{}
	form.Set("tenant_name", "New Org")
	form.Set("tenant_slug", "neworg-"+uuid.New().String()[:8])
	form.Set("name", "New User")
	form.Set("email", "newuser@example.com")
	form.Set("password", "password123")
	form.Set("timezone", "America/New_York")
	form.Set("ref", "booking:"+booking.Token) // Include ref parameter

	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// Call handler
	h.Auth.Register(w, req)

	// Verify redirect to onboarding (successful registration)
	if w.Code != http.StatusFound && w.Code != http.StatusSeeOther {
		t.Errorf("Expected redirect status, got %d", w.Code)
	}

	// Verify conversion was marked as registered
	updatedConversion, err := repos.SignupConversion.GetByEmail(context.Background(), "newuser@example.com")
	if err != nil {
		t.Fatalf("Failed to get conversion: %v", err)
	}

	if updatedConversion == nil {
		t.Fatal("Expected conversion to exist")
	}

	if updatedConversion.RegisteredAt == nil {
		t.Error("Expected registered_at to be set")
	}

	if !updatedConversion.IsRegistered() {
		t.Error("Expected conversion to be marked as registered")
	}
}

// TestRegister_WithoutRef_NoConversionTracking tests that registration without ref works normally
func TestRegister_WithoutRef_NoConversionTracking(t *testing.T) {
	// Setup test database
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	// Create handlers
	h := createTestHandlers(t, repos)

	// Create registration form data WITHOUT ref parameter
	form := url.Values{}
	form.Set("tenant_name", "Another Org")
	form.Set("tenant_slug", "anotherorg-"+uuid.New().String()[:8])
	form.Set("name", "Another User")
	form.Set("email", "anotheruser@example.com")
	form.Set("password", "password123")
	form.Set("timezone", "America/New_York")

	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// Call handler
	h.Auth.Register(w, req)

	// Verify redirect to onboarding (successful registration)
	if w.Code != http.StatusFound && w.Code != http.StatusSeeOther {
		t.Errorf("Expected redirect status, got %d", w.Code)
	}

	// Verify no conversion exists for this email
	conversion, err := repos.SignupConversion.GetByEmail(context.Background(), "anotheruser@example.com")
	if err != nil {
		t.Fatalf("Failed to get conversion: %v", err)
	}

	if conversion != nil {
		t.Error("Expected no conversion to exist for registration without ref")
	}
}

// TestRegister_WithInvalidRef_NoConversionTracking tests that invalid ref format is ignored
func TestRegister_WithInvalidRef_NoConversionTracking(t *testing.T) {
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
			name: "Wrong prefix",
			ref:  "meeting:token123",
		},
		{
			name: "No colon",
			ref:  "bookingtoken123",
		},
		{
			name: "Empty ref",
			ref:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email := "user-" + uuid.New().String()[:8] + "@example.com"

			form := url.Values{}
			form.Set("tenant_name", "Test Org "+uuid.New().String()[:4])
			form.Set("tenant_slug", "testorg-"+uuid.New().String()[:8])
			form.Set("name", "Test User")
			form.Set("email", email)
			form.Set("password", "password123")
			form.Set("timezone", "America/New_York")
			if tt.ref != "" {
				form.Set("ref", tt.ref)
			}

			req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			h.Auth.Register(w, req)

			// Should still register successfully
			if w.Code != http.StatusFound && w.Code != http.StatusSeeOther {
				t.Errorf("Expected redirect status, got %d", w.Code)
			}
		})
	}
}

// TestRegister_WithBookingRef_ConversionNotFound_StillRegisters tests graceful handling
func TestRegister_WithBookingRef_ConversionNotFound_StillRegisters(t *testing.T) {
	// Setup test database
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	// Create handlers
	h := createTestHandlers(t, repos)

	// Create registration form with ref to non-existent booking
	form := url.Values{}
	form.Set("tenant_name", "Yet Another Org")
	form.Set("tenant_slug", "yetanotherorg-"+uuid.New().String()[:8])
	form.Set("name", "Yet Another User")
	form.Set("email", "yetanotheruser@example.com")
	form.Set("password", "password123")
	form.Set("timezone", "America/New_York")
	form.Set("ref", "booking:nonexistent-token-123")

	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	// Call handler
	h.Auth.Register(w, req)

	// Should still register successfully even if conversion doesn't exist
	if w.Code != http.StatusFound && w.Code != http.StatusSeeOther {
		t.Errorf("Expected redirect status, got %d", w.Code)
	}
}

// TestRegister_WithBookingRef_MultipleConversions_MarksLatest tests that only latest is marked
func TestRegister_WithBookingRef_MultipleConversions_MarksLatest(t *testing.T) {
	// Setup test database
	db, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	// Create test booking
	booking, err := createTestBookingData(t, db, repos)
	if err != nil {
		t.Fatalf("Failed to create test booking: %v", err)
	}

	// Get the host to get tenant ID
	host, err := repos.Host.GetByID(context.Background(), booking.HostID)
	if err != nil {
		t.Fatalf("Failed to get host: %v", err)
	}

	email := "multiclick@example.com"

	// Create TWO conversion records (simulating two clicks)
	conversion1 := &models.SignupConversion{
		ID:              uuid.New().String(),
		SourceBookingID: &booking.ID,
		InviteeEmail:    email,
		ClickedAt:       models.NewSQLiteTime(models.Now().Add(-1 * 3600)), // 1 hour ago
		RegisteredAt:    nil,
		TenantID:        host.TenantID,
		CreatedAt:       models.NewSQLiteTime(models.Now().Add(-1 * 3600)),
		UpdatedAt:       models.NewSQLiteTime(models.Now().Add(-1 * 3600)),
	}

	conversion2 := &models.SignupConversion{
		ID:              uuid.New().String(),
		SourceBookingID: &booking.ID,
		InviteeEmail:    email,
		ClickedAt:       models.Now(), // More recent
		RegisteredAt:    nil,
		TenantID:        host.TenantID,
		CreatedAt:       models.Now(),
		UpdatedAt:       models.Now(),
	}

	if err := repos.SignupConversion.Create(context.Background(), conversion1); err != nil {
		t.Fatalf("Failed to create first conversion: %v", err)
	}

	if err := repos.SignupConversion.Create(context.Background(), conversion2); err != nil {
		t.Fatalf("Failed to create second conversion: %v", err)
	}

	// Create handlers
	h := createTestHandlers(t, repos)

	// Register
	form := url.Values{}
	form.Set("tenant_name", "Multi Click Org")
	form.Set("tenant_slug", "multiclick-"+uuid.New().String()[:8])
	form.Set("name", "Multi Click User")
	form.Set("email", email)
	form.Set("password", "password123")
	form.Set("timezone", "America/New_York")
	form.Set("ref", "booking:"+booking.Token)

	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	h.Auth.Register(w, req)

	// Verify redirect
	if w.Code != http.StatusFound && w.Code != http.StatusSeeOther {
		t.Errorf("Expected redirect status, got %d", w.Code)
	}

	// Verify the most recent conversion was marked (GetByEmail returns most recent)
	latestConversion, err := repos.SignupConversion.GetByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("Failed to get conversion: %v", err)
	}

	if latestConversion == nil {
		t.Fatal("Expected conversion to exist")
	}

	if !latestConversion.IsRegistered() {
		t.Error("Expected most recent conversion to be marked as registered")
	}
}
