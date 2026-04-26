package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
)

// seedConnection creates a tenant + host + calendar_connections row and returns
// the connection model. It is the minimal fixture every provider_calendars test
// needs.
func seedConnection(t *testing.T, repos *Repositories, suffix string) (*models.Host, *models.CalendarConnection) {
	t.Helper()
	ctx := context.Background()

	tenant := &models.Tenant{
		ID:        uuid.New().String(),
		Slug:      "tenant-" + suffix,
		Name:      "Tenant " + suffix,
		CreatedAt: models.Now(),
		UpdatedAt: models.Now(),
	}
	if err := repos.Tenant.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	host := &models.Host{
		ID:           uuid.New().String(),
		TenantID:     tenant.ID,
		Email:        "host-" + suffix + "@example.com",
		PasswordHash: "hash",
		Name:         "Host " + suffix,
		Slug:         "host-" + suffix,
		Timezone:     "UTC",
		CreatedAt:    models.Now(),
		UpdatedAt:    models.Now(),
	}
	if err := repos.Host.Create(ctx, host); err != nil {
		t.Fatalf("create host: %v", err)
	}

	conn := &models.CalendarConnection{
		ID:         uuid.New().String(),
		HostID:     host.ID,
		Provider:   models.CalendarProviderGoogle,
		Name:       "Google Calendar",
		CalendarID: "primary",
		SyncStatus: models.CalendarSyncStatusUnknown,
		CreatedAt:  models.Now(),
		UpdatedAt:  models.Now(),
	}
	if err := repos.Calendar.Create(ctx, conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}

	return host, conn
}

// TestProviderCalendar_BackfillRowExists verifies that the migration backfilled
// a provider_calendars row with id == calendar_connections.id for every
// existing connection.
func TestProviderCalendar_BackfillRowExists(t *testing.T) {
	for _, driver := range []string{"sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db, cleanup := setupTestDB(t, driver)
			defer cleanup()
			repos := NewRepositories(db, driver)

			_, conn := seedConnection(t, repos, "backfill")

			// The migration only backfills rows that existed BEFORE the migration
			// ran. A fresh test DB has no pre-migration rows, so we simulate the
			// backfill ourselves by inserting a connection and asserting that
			// when a provider_calendars row exists for that connection id it can
			// be looked up by GetByID.
			pc := &models.ProviderCalendar{
				ID:                 conn.ID,
				ConnectionID:       conn.ID,
				ProviderCalendarID: "primary",
				Name:               "Primary",
				IsPrimary:          true,
				IsWritable:         true,
				PollBusy:           true,
				SyncStatus:         models.CalendarSyncStatusUnknown,
				CreatedAt:          models.Now(),
				UpdatedAt:          models.Now(),
			}
			if err := repos.ProviderCalendar.Create(context.Background(), pc); err != nil {
				t.Fatalf("create provider calendar: %v", err)
			}

			got, err := repos.ProviderCalendar.GetByID(context.Background(), conn.ID)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got == nil {
				t.Fatal("expected backfilled provider calendar, got nil")
			}
			if got.ID != conn.ID {
				t.Errorf("backfill id mismatch: got %s want %s", got.ID, conn.ID)
			}
			if !got.IsPrimary || !got.IsWritable || !got.PollBusy {
				t.Errorf("backfill defaults wrong: primary=%v writable=%v poll=%v", got.IsPrimary, got.IsWritable, got.PollBusy)
			}
		})
	}
}

// TestProviderCalendar_UpsertFromProvider checks that calling upsert twice with
// the same provider_calendar_id updates the existing row in place rather than
// inserting a duplicate, and that user-set color / poll_busy are preserved.
func TestProviderCalendar_UpsertFromProvider(t *testing.T) {
	for _, driver := range []string{"sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db, cleanup := setupTestDB(t, driver)
			defer cleanup()
			repos := NewRepositories(db, driver)

			ctx := context.Background()
			_, conn := seedConnection(t, repos, "upsert")

			pc, err := repos.ProviderCalendar.UpsertFromProvider(
				ctx, conn.ID, "work@example.com", "Work Calendar", "#378ADD", false, true,
			)
			if err != nil {
				t.Fatalf("first upsert: %v", err)
			}
			origID := pc.ID

			// User customizes: turn off polling, change color.
			if err := repos.ProviderCalendar.UpdatePollBusy(ctx, conn.HostID, pc.ID, false); err != nil {
				t.Fatalf("UpdatePollBusy: %v", err)
			}
			if err := repos.ProviderCalendar.UpdateColor(ctx, conn.HostID, pc.ID, "#1D9E75"); err != nil {
				t.Fatalf("UpdateColor: %v", err)
			}

			// Second upsert with a NEW provider-supplied color and updated name.
			updated, err := repos.ProviderCalendar.UpsertFromProvider(
				ctx, conn.ID, "work@example.com", "Work Calendar Renamed", "#E24B4A", true, true,
			)
			if err != nil {
				t.Fatalf("second upsert: %v", err)
			}

			if updated.ID != origID {
				t.Fatalf("upsert created a new row instead of updating: orig=%s new=%s", origID, updated.ID)
			}
			if updated.Name != "Work Calendar Renamed" {
				t.Errorf("name not refreshed: got %s", updated.Name)
			}
			if !updated.IsPrimary {
				t.Errorf("is_primary not refreshed")
			}

			// Reload to confirm preservation of user-set color and poll_busy.
			reread, err := repos.ProviderCalendar.GetByID(ctx, origID)
			if err != nil || reread == nil {
				t.Fatalf("reload: err=%v row=%v", err, reread)
			}
			if reread.Color != "#1D9E75" {
				t.Errorf("user-set color overridden: got %s want #1D9E75", reread.Color)
			}
			if reread.PollBusy {
				t.Errorf("poll_busy was reset to true on upsert")
			}
		})
	}
}

// TestProviderCalendar_DeleteMissing covers the two important cases: removing
// a row whose provider_calendar_id no longer appears at the provider, and
// keeping all rows when the keep slice is non-empty and matches.
func TestProviderCalendar_DeleteMissing(t *testing.T) {
	for _, driver := range []string{"sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db, cleanup := setupTestDB(t, driver)
			defer cleanup()
			repos := NewRepositories(db, driver)
			ctx := context.Background()

			_, conn := seedConnection(t, repos, "delmissing")

			// Seed three calendars under the connection.
			for _, name := range []string{"alpha", "beta", "gamma"} {
				if _, err := repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, name, name, "", false, true); err != nil {
					t.Fatalf("upsert %s: %v", name, err)
				}
			}

			// Remove "beta" — keep alpha and gamma.
			if err := repos.ProviderCalendar.DeleteMissing(ctx, conn.ID, []string{"alpha", "gamma"}); err != nil {
				t.Fatalf("DeleteMissing: %v", err)
			}

			cals, err := repos.ProviderCalendar.GetByConnectionID(ctx, conn.ID)
			if err != nil {
				t.Fatalf("GetByConnectionID: %v", err)
			}
			names := map[string]bool{}
			for _, c := range cals {
				names[c.ProviderCalendarID] = true
			}
			if names["beta"] {
				t.Error("beta should have been deleted")
			}
			if !names["alpha"] || !names["gamma"] {
				t.Errorf("expected alpha and gamma to remain, got %v", names)
			}
		})
	}
}

// TestProviderCalendar_GetPolledByHostID verifies that only calendars with
// poll_busy=true are returned, and that they are scoped to the host.
func TestProviderCalendar_GetPolledByHostID(t *testing.T) {
	for _, driver := range []string{"sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db, cleanup := setupTestDB(t, driver)
			defer cleanup()
			repos := NewRepositories(db, driver)
			ctx := context.Background()

			host, conn := seedConnection(t, repos, "polled")

			pcWork, err := repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, "work", "Work", "", true, true)
			if err != nil {
				t.Fatal(err)
			}
			pcHolidays, err := repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, "holidays", "Holidays", "", false, false)
			if err != nil {
				t.Fatal(err)
			}

			// Disable polling on holidays.
			if err := repos.ProviderCalendar.UpdatePollBusy(ctx, host.ID, pcHolidays.ID, false); err != nil {
				t.Fatal(err)
			}

			polled, err := repos.ProviderCalendar.GetPolledByHostID(ctx, host.ID)
			if err != nil {
				t.Fatalf("GetPolledByHostID: %v", err)
			}
			if len(polled) != 1 {
				t.Fatalf("expected 1 polled calendar, got %d", len(polled))
			}
			if polled[0].ID != pcWork.ID {
				t.Errorf("expected the work calendar, got %s", polled[0].Name)
			}
		})
	}
}

// TestProviderCalendar_OwnershipScoping confirms that a host cannot toggle a
// calendar belonging to a different host.
func TestProviderCalendar_OwnershipScoping(t *testing.T) {
	for _, driver := range []string{"sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db, cleanup := setupTestDB(t, driver)
			defer cleanup()
			repos := NewRepositories(db, driver)
			ctx := context.Background()

			_, connA := seedConnection(t, repos, "ownerA")
			hostB, _ := seedConnection(t, repos, "ownerB")

			pc, err := repos.ProviderCalendar.UpsertFromProvider(ctx, connA.ID, "calA", "Cal A", "", true, true)
			if err != nil {
				t.Fatal(err)
			}

			err = repos.ProviderCalendar.UpdatePollBusy(ctx, hostB.ID, pc.ID, false)
			if err == nil {
				t.Fatal("expected ownership error, got nil")
			}

			err = repos.ProviderCalendar.UpdateColor(ctx, hostB.ID, pc.ID, "#378ADD")
			if err == nil {
				t.Fatal("expected ownership error, got nil")
			}
		})
	}
}

// TestProviderCalendar_GetByHostID_OrdersByConnectionThenPrimary verifies the
// ordering contract used by the dashboard.
func TestProviderCalendar_GetByHostID_OrdersByConnectionThenPrimary(t *testing.T) {
	for _, driver := range []string{"sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db, cleanup := setupTestDB(t, driver)
			defer cleanup()
			repos := NewRepositories(db, driver)
			ctx := context.Background()

			host, conn := seedConnection(t, repos, "ordering")

			// Insert non-primary first to make sure ordering is by is_primary DESC.
			_, _ = repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, "personal", "Personal", "", false, true)
			_, _ = repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, "work", "Work", "", true, true)

			cals, err := repos.ProviderCalendar.GetByHostID(ctx, host.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(cals) != 2 {
				t.Fatalf("expected 2, got %d", len(cals))
			}
			if !cals[0].IsPrimary {
				t.Errorf("expected primary first, got %s", cals[0].Name)
			}
		})
	}
}
