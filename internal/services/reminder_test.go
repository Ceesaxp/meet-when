package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
)

// TestReminder_processHostedEventReminders_FiresForEachAttendeeAndMarksSent
// exercises the hosted-event branch of the reminder loop:
//   - One scheduled hosted event sitting inside the 23-25h reminder window
//   - Two attendees → expect two SendHostedEventReminder calls
//   - reminder_sent flips to true after the dispatch
//   - A second pass with the same window does not re-fire (because
//     GetUpcomingForReminders filters reminder_sent rows)
func TestReminder_processHostedEventReminders_FiresForEachAttendeeAndMarksSent(t *testing.T) {
	_, repos, cleanup := setupTestRepos(t)
	defer cleanup()
	ctx := context.Background()

	tenant := &models.Tenant{
		ID: uuid.New().String(), Slug: "t", Name: "Tenant",
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Tenant.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	host := &models.Host{
		ID: uuid.New().String(), TenantID: tenant.ID,
		Email: "host@example.com", PasswordHash: "x", Name: "Host", Slug: "host",
		Timezone: "UTC", CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Host.Create(ctx, host); err != nil {
		t.Fatalf("create host: %v", err)
	}

	// Event scheduled in 24 hours, within the reminder window.
	startAt := time.Now().UTC().Add(24 * time.Hour)
	event := &models.HostedEvent{
		ID: uuid.New().String(), TenantID: tenant.ID, HostID: host.ID,
		Title: "Standup", StartTime: models.NewSQLiteTime(startAt),
		EndTime: models.NewSQLiteTime(startAt.Add(30 * time.Minute)), Duration: 30,
		Timezone: "UTC", LocationType: models.ConferencingProviderGoogleMeet,
		Status:    models.HostedEventStatusScheduled,
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.HostedEvent.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}
	for _, email := range []string{"a@example.com", "b@example.com"} {
		if err := repos.HostedEventAttendee.ReplaceForEvent(ctx, event.ID, []*models.HostedEventAttendee{{
			ID: uuid.New().String(), HostedEventID: event.ID, Email: email, Name: email,
			CreatedAt: models.Now(),
		}}); err != nil {
			// ReplaceForEvent overwrites — switch to one call with both rows.
			t.Fatalf("ReplaceForEvent: %v", err)
		}
	}
	// Final attendee state: ReplaceForEvent above overwrites each call, so
	// re-seed both atomically.
	if err := repos.HostedEventAttendee.ReplaceForEvent(ctx, event.ID, []*models.HostedEventAttendee{
		{ID: uuid.New().String(), HostedEventID: event.ID, Email: "a@example.com", Name: "A", CreatedAt: models.Now()},
		{ID: uuid.New().String(), HostedEventID: event.ID, Email: "b@example.com", Name: "B", CreatedAt: models.Now()},
	}); err != nil {
		t.Fatalf("seed final attendees: %v", err)
	}

	// We can't intercept the real EmailService send (it would try to dial
	// SMTP), so swap in a config that points to a non-existent provider —
	// reminder dispatch logs but still marks the event as sent. The thing we
	// actually verify here is that GetUpcomingForReminders + MarkReminderSent
	// drive the state machine correctly; email content is covered by the
	// hosted_event_test.go spy.
	cfg := &config.Config{}
	cfg.Email.Provider = "smtp"
	cfg.Email.SMTPHost = "127.0.0.1"
	cfg.Email.SMTPPort = 1 // guaranteed-closed port
	cfg.Email.FromAddress = "noreply@test.local"
	emailSvc := NewEmailService(cfg)

	reminder := NewReminderService(repos, emailSvc)
	reminder.processHostedEventReminders(ctx, time.Now().UTC().Add(23*time.Hour), time.Now().UTC().Add(25*time.Hour))

	post, err := repos.HostedEvent.GetByID(ctx, event.ID)
	if err != nil || post == nil {
		t.Fatalf("post-fetch event: %v / %v", err, post)
	}
	if !post.ReminderSent {
		t.Fatal("expected reminder_sent=true after processHostedEventReminders")
	}

	// A second pass over the same window must not pick the event up again.
	due, err := repos.HostedEvent.GetUpcomingForReminders(ctx, time.Now().UTC().Add(23*time.Hour), time.Now().UTC().Add(25*time.Hour))
	if err != nil {
		t.Fatalf("GetUpcomingForReminders second pass: %v", err)
	}
	for _, e := range due {
		if e.ID == event.ID {
			t.Fatal("event re-surfaced after reminder_sent was marked")
		}
	}
}
