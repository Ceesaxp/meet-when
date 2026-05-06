package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
)

func TestHostedEventRepository_RoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		driver string
	}{
		{"PostgreSQL", "postgres"},
		{"SQLite", "sqlite"},
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

			tenantID, hostID := seedHostedEventParents(t, repos)

			start := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
			end := start.Add(30 * time.Minute)
			templateID := "" // nullable; pass nil
			event := &models.HostedEvent{
				ID:           uuid.New().String(),
				TenantID:     tenantID,
				HostID:       hostID,
				TemplateID:   nullableID(templateID),
				Title:        "Coffee chat",
				Description:  "intro",
				StartTime:    models.NewSQLiteTime(start),
				EndTime:      models.NewSQLiteTime(end),
				Duration:     30,
				Timezone:     "UTC",
				LocationType: models.ConferencingProviderGoogleMeet,
				Status:       models.HostedEventStatusScheduled,
				CreatedAt:    models.Now(),
				UpdatedAt:    models.Now(),
			}
			if err := repos.HostedEvent.Create(ctx, event); err != nil {
				t.Fatalf("Create: %v", err)
			}

			got, err := repos.HostedEvent.GetByID(ctx, event.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got == nil || got.Title != "Coffee chat" {
				t.Fatalf("unexpected event: %+v", got)
			}

			// ListByHost should return it.
			list, err := repos.HostedEvent.ListByHost(ctx, hostID, false)
			if err != nil {
				t.Fatalf("ListByHost: %v", err)
			}
			if len(list) != 1 {
				t.Fatalf("expected 1 event, got %d", len(list))
			}

			// GetByHostIDAndTimeRange should overlap.
			overlapping, err := repos.HostedEvent.GetByHostIDAndTimeRange(ctx, hostID, start.Add(-time.Hour), end.Add(time.Hour), nil)
			if err != nil {
				t.Fatalf("GetByHostIDAndTimeRange: %v", err)
			}
			if len(overlapping) != 1 {
				t.Fatalf("expected 1 overlapping, got %d", len(overlapping))
			}

			// excludeEventID filter.
			selfExcluded, err := repos.HostedEvent.GetByHostIDAndTimeRange(ctx, hostID, start.Add(-time.Hour), end.Add(time.Hour), &event.ID)
			if err != nil {
				t.Fatalf("GetByHostIDAndTimeRange (exclude): %v", err)
			}
			if len(selfExcluded) != 0 {
				t.Fatalf("expected 0 when excluding self, got %d", len(selfExcluded))
			}

			// MarkReminderSent.
			if err := repos.HostedEvent.MarkReminderSent(ctx, event.ID); err != nil {
				t.Fatalf("MarkReminderSent: %v", err)
			}
			fresh, _ := repos.HostedEvent.GetByID(ctx, event.ID)
			if !fresh.ReminderSent {
				t.Fatal("ReminderSent not persisted")
			}
		})
	}
}

func TestHostedEventAttendeeRepository_ReplaceForEvent(t *testing.T) {
	tests := []struct {
		name   string
		driver string
	}{
		{"PostgreSQL", "postgres"},
		{"SQLite", "sqlite"},
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

			tenantID, hostID := seedHostedEventParents(t, repos)
			event := &models.HostedEvent{
				ID: uuid.New().String(), TenantID: tenantID, HostID: hostID,
				Title: "x", Duration: 30, Timezone: "UTC",
				StartTime: models.Now(), EndTime: models.Now(),
				LocationType: models.ConferencingProviderGoogleMeet,
				Status:       models.HostedEventStatusScheduled,
				CreatedAt:    models.Now(), UpdatedAt: models.Now(),
			}
			if err := repos.HostedEvent.Create(ctx, event); err != nil {
				t.Fatalf("event create: %v", err)
			}

			first := []*models.HostedEventAttendee{
				{ID: uuid.New().String(), Email: "a@example.com", Name: "A", CreatedAt: models.Now()},
				{ID: uuid.New().String(), Email: "b@example.com", Name: "B", CreatedAt: models.Now()},
			}
			if err := repos.HostedEventAttendee.ReplaceForEvent(ctx, event.ID, first); err != nil {
				t.Fatalf("Replace 1: %v", err)
			}

			got, err := repos.HostedEventAttendee.ListByEvent(ctx, event.ID)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != 2 {
				t.Fatalf("expected 2 attendees, got %d", len(got))
			}

			// Replace with a different set; previous rows must be gone.
			second := []*models.HostedEventAttendee{
				{ID: uuid.New().String(), Email: "c@example.com", Name: "C", CreatedAt: models.Now()},
			}
			if err := repos.HostedEventAttendee.ReplaceForEvent(ctx, event.ID, second); err != nil {
				t.Fatalf("Replace 2: %v", err)
			}
			got2, _ := repos.HostedEventAttendee.ListByEvent(ctx, event.ID)
			if len(got2) != 1 || got2[0].Email != "c@example.com" {
				t.Fatalf("unexpected after replace 2: %+v", got2)
			}
		})
	}
}

func nullableID(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// seedHostedEventParents inserts the minimum tenant + host needed to satisfy
// hosted_events foreign keys. Returns the IDs.
func seedHostedEventParents(t *testing.T, repos *Repositories) (tenantID, hostID string) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.New().String()[:8]

	tenant := &models.Tenant{
		ID: uuid.New().String(), Slug: "t-" + suffix, Name: "Tenant",
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Tenant.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	host := &models.Host{
		ID: uuid.New().String(), TenantID: tenant.ID,
		Email: "h-" + suffix + "@example.com", PasswordHash: "x",
		Name: "Host", Slug: "host-" + suffix, Timezone: "UTC",
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Host.Create(ctx, host); err != nil {
		t.Fatalf("create host: %v", err)
	}
	return tenant.ID, host.ID
}
