package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
)

func TestBookingRepository_Update_PersistsTimeFields(t *testing.T) {
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

			repos := NewRepositories(db, tt.driver)

			ctx := context.Background()
			suffix := uuid.New().String()[:8]

			// Create tenant
			tenant := &models.Tenant{
				ID:        uuid.New().String(),
				Slug:      "test-tenant-" + suffix,
				Name:      "Test Tenant",
				CreatedAt: models.Now(),
				UpdatedAt: models.Now(),
			}
			if err := repos.Tenant.Create(ctx, tenant); err != nil {
				t.Fatalf("failed to create tenant: %v", err)
			}

			// Create host
			host := &models.Host{
				ID:           uuid.New().String(),
				TenantID:     tenant.ID,
				Email:        "host-" + suffix + "@example.com",
				PasswordHash: "hash",
				Name:         "Test Host",
				Slug:         "host-" + suffix,
				Timezone:     "UTC",
				CreatedAt:    models.Now(),
				UpdatedAt:    models.Now(),
			}
			if err := repos.Host.Create(ctx, host); err != nil {
				t.Fatalf("failed to create host: %v", err)
			}

			// Create template
			tmpl := &models.MeetingTemplate{
				ID:        uuid.New().String(),
				HostID:    host.ID,
				Slug:      "test-template-" + suffix,
				Name:      "Test Template",
				CreatedAt: models.Now(),
				UpdatedAt: models.Now(),
			}
			if err := repos.Template.Create(ctx, tmpl); err != nil {
				t.Fatalf("failed to create template: %v", err)
			}

			// Create booking with initial times
			originalStart := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
			originalEnd := originalStart.Add(60 * time.Minute)
			originalDuration := 60

			booking := &models.Booking{
				ID:           uuid.New().String(),
				TemplateID:   tmpl.ID,
				HostID:       host.ID,
				Token:        "test-token-" + suffix,
				Status:       models.BookingStatusPending,
				StartTime:    models.NewSQLiteTime(originalStart),
				EndTime:      models.NewSQLiteTime(originalEnd),
				Duration:     originalDuration,
				InviteeName:  "Invitee",
				InviteeEmail: "invitee@example.com",
				CreatedAt:    models.Now(),
				UpdatedAt:    models.Now(),
			}
			if err := repos.Booking.Create(ctx, booking); err != nil {
				t.Fatalf("failed to create booking: %v", err)
			}

			// Simulate rescheduling: update time fields
			newStart := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
			newEnd := newStart.Add(30 * time.Minute)
			newDuration := 30

			booking.StartTime = models.NewSQLiteTime(newStart)
			booking.EndTime = models.NewSQLiteTime(newEnd)
			booking.Duration = newDuration
			booking.UpdatedAt = models.Now()

			err := repos.Booking.Update(ctx, booking)
			if err != nil {
				t.Fatalf("Update failed: %v", err)
			}

			// Re-read booking from DB
			updated, err := repos.Booking.GetByID(ctx, booking.ID)
			if err != nil {
				t.Fatalf("GetByID failed: %v", err)
			}
			if updated == nil {
				t.Fatal("expected booking, got nil")
			}

			// Verify the new times were persisted (not the originals)
			if updated.StartTime.UTC().Truncate(time.Second).Equal(originalStart) {
				t.Error("start_time was NOT updated — still matches original time")
			}
			if updated.EndTime.UTC().Truncate(time.Second).Equal(originalEnd) {
				t.Error("end_time was NOT updated — still matches original time")
			}
			if updated.Duration == originalDuration {
				t.Errorf("duration was NOT updated — still %d", originalDuration)
			}

			// Verify the new times match what we set
			if !updated.StartTime.UTC().Truncate(time.Second).Equal(newStart) {
				t.Errorf("expected start_time %v, got %v", newStart, updated.StartTime.UTC().Truncate(time.Second))
			}
			if !updated.EndTime.UTC().Truncate(time.Second).Equal(newEnd) {
				t.Errorf("expected end_time %v, got %v", newEnd, updated.EndTime.UTC().Truncate(time.Second))
			}
			if updated.Duration != newDuration {
				t.Errorf("expected duration %d, got %d", newDuration, updated.Duration)
			}
			if updated.Status != models.BookingStatusPending {
				t.Errorf("expected status %s, got %s", models.BookingStatusPending, updated.Status)
			}
		})
	}
}
