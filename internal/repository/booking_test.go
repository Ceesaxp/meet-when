package repository

import (
	"context"
	"testing"
	"time"

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

			repo := &BookingRepository{db: db, driver: tt.driver}

			// Create a booking with initial times
			_, _, booking := createTestBooking(t, db, tt.driver)

			originalStart := booking.StartTime
			originalEnd := booking.EndTime
			originalDuration := booking.Duration

			// Simulate rescheduling: update time fields
			newStart := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
			newEnd := newStart.Add(30 * time.Minute)
			newDuration := 30

			booking.StartTime = models.NewSQLiteTime(newStart)
			booking.EndTime = models.NewSQLiteTime(newEnd)
			booking.Duration = newDuration
			booking.UpdatedAt = models.Now()
			booking.Status = models.BookingStatusPending

			err := repo.Update(context.Background(), booking)
			if err != nil {
				t.Fatalf("Update failed: %v", err)
			}

			// Re-read booking from DB
			updated, err := repo.GetByID(context.Background(), booking.ID)
			if err != nil {
				t.Fatalf("GetByID failed: %v", err)
			}
			if updated == nil {
				t.Fatal("expected booking, got nil")
			}

			// Verify the new times were persisted (not the originals)
			if updated.StartTime.UTC().Equal(originalStart.UTC()) {
				t.Error("start_time was NOT updated — still matches original time")
			}
			if updated.EndTime.UTC().Equal(originalEnd.UTC()) {
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
