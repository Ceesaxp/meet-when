package handlers

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/meet-when/meet-when/internal/models"
)

// TestBookingStatusTemplate_SignupCTA verifies the signup CTA is rendered correctly
func TestBookingStatusTemplate_SignupCTA(t *testing.T) {
	tests := []struct {
		name           string
		bookingStatus  models.BookingStatus
		expectSignupCTA bool
	}{
		{
			name:           "Confirmed booking shows signup CTA",
			bookingStatus:  models.BookingStatusConfirmed,
			expectSignupCTA: true,
		},
		{
			name:           "Pending booking shows signup CTA",
			bookingStatus:  models.BookingStatusPending,
			expectSignupCTA: true,
		},
		{
			name:           "Cancelled booking does not show signup CTA",
			bookingStatus:  models.BookingStatusCancelled,
			expectSignupCTA: false,
		},
		{
			name:           "Rejected booking does not show signup CTA",
			bookingStatus:  models.BookingStatusRejected,
			expectSignupCTA: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the template
			tmpl, err := template.New("booking_status.html").Funcs(template.FuncMap{
				"formatDateTime": func(t models.SQLiteTime) string {
					return "2026-01-29 10:00 AM"
				},
			}).ParseFiles("../../templates/pages/booking_status.html")
			if err != nil {
				t.Fatalf("Failed to parse template: %v", err)
			}

			// Create test data
			data := PageData{
				Title: "Test Booking",
				Tenant: &models.Tenant{
					Slug: "test-tenant",
					Name: "Test Tenant",
				},
				Host: &models.Host{
					Slug: "test-host",
					Name: "Test Host",
				},
				Data: map[string]interface{}{
					"Booking": &models.Booking{
						Token:  "test-token-123",
						Status: tt.bookingStatus,
						StartTime: models.Now(),
						Duration: 30,
					},
					"Template": &models.MeetingTemplate{
						Name:         "Test Meeting",
						LocationType: models.ConferencingProviderGoogleMeet,
					},
				},
			}

			// Render the template
			var buf bytes.Buffer
			err = tmpl.Execute(&buf, data)
			if err != nil {
				t.Fatalf("Failed to execute template: %v", err)
			}

			output := buf.String()

			// Verify signup CTA section presence
			hasSignupPrompt := strings.Contains(output, "signup-prompt")
			hasHeadline := strings.Contains(output, "Need your own scheduling page?")
			hasSubtext := strings.Contains(output, "Create a free MeetWhen account")
			hasSignupLink := strings.Contains(output, "/signup/track?ref=booking:test-token-123")

			if tt.expectSignupCTA {
				// Should have all signup CTA elements
				if !hasSignupPrompt {
					t.Error("Expected signup-prompt class to be present for status:", tt.bookingStatus)
				}
				if !hasHeadline {
					t.Error("Expected signup CTA headline to be present for status:", tt.bookingStatus)
				}
				if !hasSubtext {
					t.Error("Expected signup CTA subtext to be present for status:", tt.bookingStatus)
				}
				if !hasSignupLink {
					t.Error("Expected signup tracking link to be present for status:", tt.bookingStatus)
				}

				// Verify placement: should be after secondary-actions and before back-link
				secondaryActionsPos := strings.Index(output, "secondary-actions")
				signupPromptPos := strings.Index(output, "signup-prompt")
				backLinkPos := strings.Index(output, "back-link")

				if secondaryActionsPos == -1 {
					t.Error("Could not find secondary-actions section")
				}
				if signupPromptPos == -1 {
					t.Error("Could not find signup-prompt section")
				}
				if backLinkPos == -1 {
					t.Error("Could not find back-link section")
				}

				// Verify correct order
				if secondaryActionsPos > signupPromptPos {
					t.Error("signup-prompt should appear AFTER secondary-actions")
				}
				if signupPromptPos > backLinkPos {
					t.Error("signup-prompt should appear BEFORE back-link")
				}
			} else {
				// Should NOT have signup CTA elements
				if hasSignupPrompt {
					t.Error("Did not expect signup-prompt class for status:", tt.bookingStatus)
				}
				if hasHeadline {
					t.Error("Did not expect signup CTA headline for status:", tt.bookingStatus)
				}
				if hasSubtext {
					t.Error("Did not expect signup CTA subtext for status:", tt.bookingStatus)
				}
				if hasSignupLink {
					t.Error("Did not expect signup tracking link for status:", tt.bookingStatus)
				}
			}
		})
	}
}

// TestBookingStatusTemplate_SignupCTA_ButtonStyle verifies the CTA button has correct styling
func TestBookingStatusTemplate_SignupCTA_ButtonStyle(t *testing.T) {
	// Parse the template
	tmpl, err := template.New("booking_status.html").Funcs(template.FuncMap{
		"formatDateTime": func(t models.SQLiteTime) string {
			return "2026-01-29 10:00 AM"
		},
	}).ParseFiles("../../templates/pages/booking_status.html")
	if err != nil {
		t.Fatalf("Failed to parse template: %v", err)
	}

	// Create test data with confirmed status
	data := PageData{
		Title: "Test Booking",
		Tenant: &models.Tenant{
			Slug: "test-tenant",
			Name: "Test Tenant",
		},
		Host: &models.Host{
			Slug: "test-host",
			Name: "Test Host",
		},
		Data: map[string]interface{}{
			"Booking": &models.Booking{
				Token:     "test-token-123",
				Status:    models.BookingStatusConfirmed,
				StartTime: models.Now(),
				Duration:  30,
			},
			"Template": &models.MeetingTemplate{
				Name:         "Test Meeting",
				LocationType: models.ConferencingProviderGoogleMeet,
			},
		},
	}

	// Render the template
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify button has correct classes
	hasBtnAccent := strings.Contains(output, "btn-accent")
	if !hasBtnAccent {
		t.Error("Expected signup CTA button to have btn-accent class")
	}

	// Verify button is a link (not a form button)
	// Look for <a href="/signup/track?ref=booking:..." class="btn btn-accent">
	expectedPattern := `<a href="/signup/track?ref=booking:test-token-123"`
	if !strings.Contains(output, expectedPattern) {
		t.Error("Expected signup CTA to be an anchor tag with correct href")
	}
}

// TestBookingStatusTemplate_NoSignupCTA_WhenCancelled verifies no CTA for cancelled bookings
func TestBookingStatusTemplate_NoSignupCTA_WhenCancelled(t *testing.T) {
	// Parse the template
	tmpl, err := template.New("booking_status.html").Funcs(template.FuncMap{
		"formatDateTime": func(t models.SQLiteTime) string {
			return "2026-01-29 10:00 AM"
		},
	}).ParseFiles("../../templates/pages/booking_status.html")
	if err != nil {
		t.Fatalf("Failed to parse template: %v", err)
	}

	// Test with cancelled status
	data := PageData{
		Title: "Test Booking",
		Tenant: &models.Tenant{
			Slug: "test-tenant",
			Name: "Test Tenant",
		},
		Host: &models.Host{
			Slug: "test-host",
			Name: "Test Host",
		},
		Data: map[string]interface{}{
			"Booking": &models.Booking{
				Token:     "test-token-123",
				Status:    models.BookingStatusCancelled,
				StartTime: models.Now(),
				Duration:  30,
			},
			"Template": &models.MeetingTemplate{
				Name:         "Test Meeting",
				LocationType: models.ConferencingProviderGoogleMeet,
			},
		},
	}

	// Render the template
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	output := buf.String()

	// Verify no signup elements present
	if strings.Contains(output, "signup-prompt") {
		t.Error("Cancelled bookings should not show signup CTA")
	}
	if strings.Contains(output, "Need your own scheduling page?") {
		t.Error("Cancelled bookings should not show signup CTA headline")
	}
}
