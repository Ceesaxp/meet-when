package models

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSignupConversion_JSONMarshal verifies SignupConversion can be marshaled to JSON
func TestSignupConversion_JSONMarshal(t *testing.T) {
	now := Now()
	sourceBookingID := "booking-123"
	registeredAt := Now()

	conversion := SignupConversion{
		ID:              "conv-123",
		SourceBookingID: &sourceBookingID,
		InviteeEmail:    "test@example.com",
		ClickedAt:       now,
		RegisteredAt:    &registeredAt,
		TenantID:        "tenant-123",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Marshal to JSON
	data, err := json.Marshal(conversion)
	if err != nil {
		t.Fatalf("failed to marshal SignupConversion: %v", err)
	}

	// Unmarshal back
	var unmarshaled SignupConversion
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("failed to unmarshal SignupConversion: %v", err)
	}

	// Verify fields
	if unmarshaled.ID != conversion.ID {
		t.Errorf("expected ID %s, got %s", conversion.ID, unmarshaled.ID)
	}
	if unmarshaled.InviteeEmail != conversion.InviteeEmail {
		t.Errorf("expected InviteeEmail %s, got %s", conversion.InviteeEmail, unmarshaled.InviteeEmail)
	}
	if unmarshaled.TenantID != conversion.TenantID {
		t.Errorf("expected TenantID %s, got %s", conversion.TenantID, unmarshaled.TenantID)
	}
	if unmarshaled.SourceBookingID == nil || *unmarshaled.SourceBookingID != *conversion.SourceBookingID {
		t.Errorf("expected SourceBookingID %v, got %v", conversion.SourceBookingID, unmarshaled.SourceBookingID)
	}
	if unmarshaled.RegisteredAt == nil || !unmarshaled.RegisteredAt.Equal(conversion.RegisteredAt.Time) {
		t.Errorf("expected RegisteredAt %v, got %v", conversion.RegisteredAt, unmarshaled.RegisteredAt)
	}
}

// TestSignupConversion_NullableFields verifies nullable fields work correctly
func TestSignupConversion_NullableFields(t *testing.T) {
	now := Now()

	conversion := SignupConversion{
		ID:              "conv-123",
		SourceBookingID: nil, // Nullable
		InviteeEmail:    "test@example.com",
		ClickedAt:       now,
		RegisteredAt:    nil, // Nullable - not yet registered
		TenantID:        "tenant-123",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Marshal to JSON
	data, err := json.Marshal(conversion)
	if err != nil {
		t.Fatalf("failed to marshal SignupConversion: %v", err)
	}

	// Unmarshal back
	var unmarshaled SignupConversion
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("failed to unmarshal SignupConversion: %v", err)
	}

	// Verify nullable fields are nil
	if unmarshaled.SourceBookingID != nil {
		t.Errorf("expected SourceBookingID to be nil, got %v", unmarshaled.SourceBookingID)
	}
	if unmarshaled.RegisteredAt != nil {
		t.Errorf("expected RegisteredAt to be nil, got %v", unmarshaled.RegisteredAt)
	}
}

// TestSignupConversion_SQLiteTimeParsing verifies SQLiteTime fields parse correctly
func TestSignupConversion_SQLiteTimeParsing(t *testing.T) {
	// Create a time in a specific timezone
	loc, _ := time.LoadLocation("America/New_York")
	localTime := time.Date(2026, 1, 29, 15, 30, 0, 0, loc)

	conversion := SignupConversion{
		ID:           "conv-123",
		InviteeEmail: "test@example.com",
		ClickedAt:    NewSQLiteTime(localTime),
		TenantID:     "tenant-123",
		CreatedAt:    Now(),
		UpdatedAt:    Now(),
	}

	// Verify time is normalized to UTC
	if conversion.ClickedAt.Location() != time.UTC {
		t.Errorf("expected ClickedAt to be in UTC, got %v", conversion.ClickedAt.Location())
	}

	// Verify the time value is correct (should be 20:30 UTC)
	expectedUTC := localTime.UTC()
	if !conversion.ClickedAt.Equal(expectedUTC) {
		t.Errorf("expected ClickedAt %v, got %v", expectedUTC, conversion.ClickedAt.Time)
	}
}

// TestSignupConversion_IsRegistered verifies helper method for registration status
func TestSignupConversion_IsRegistered(t *testing.T) {
	tests := []struct {
		name         string
		registeredAt *SQLiteTime
		want         bool
	}{
		{
			name:         "Not registered",
			registeredAt: nil,
			want:         false,
		},
		{
			name: "Registered",
			registeredAt: func() *SQLiteTime {
				t := Now()
				return &t
			}(),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conversion := SignupConversion{
				ID:           "conv-123",
				InviteeEmail: "test@example.com",
				ClickedAt:    Now(),
				RegisteredAt: tt.registeredAt,
				TenantID:     "tenant-123",
				CreatedAt:    Now(),
				UpdatedAt:    Now(),
			}

			got := conversion.IsRegistered()
			if got != tt.want {
				t.Errorf("IsRegistered() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSignupConversion_ConversionTime verifies helper method for conversion duration
func TestSignupConversion_ConversionTime(t *testing.T) {
	clickedTime := time.Date(2026, 1, 29, 10, 0, 0, 0, time.UTC)
	registeredTime := time.Date(2026, 1, 29, 10, 15, 0, 0, time.UTC)

	registeredAt := NewSQLiteTime(registeredTime)
	conversion := SignupConversion{
		ID:           "conv-123",
		InviteeEmail: "test@example.com",
		ClickedAt:    NewSQLiteTime(clickedTime),
		RegisteredAt: &registeredAt,
		TenantID:     "tenant-123",
		CreatedAt:    Now(),
		UpdatedAt:    Now(),
	}

	duration := conversion.ConversionTime()
	if duration == nil {
		t.Fatal("expected non-nil duration for registered conversion")
	}

	expectedDuration := 15 * time.Minute
	if *duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, *duration)
	}

	// Test with unregistered conversion
	conversion.RegisteredAt = nil
	duration = conversion.ConversionTime()
	if duration != nil {
		t.Errorf("expected nil duration for unregistered conversion, got %v", duration)
	}
}
