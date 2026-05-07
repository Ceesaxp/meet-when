package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

// spyEmailSender records every hosted-event email call. Concurrent-safe so
// the goroutines inside SendHostedEventInvited et al don't race with the
// test asserting on counts.
type spyEmailSender struct {
	mu sync.Mutex

	invited          []emailCall
	updated          []emailCall
	cancelled        []emailCall
	cancelledForAttn []emailCall
	reminders        []emailCall
}

type emailCall struct {
	EventID       string
	AttendeeEmail string
	ChangedFields []string
}

func (s *spyEmailSender) SendHostedEventInvited(_ context.Context, e *models.HostedEvent, a *models.HostedEventAttendee, _ *models.Host, _ *models.Tenant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invited = append(s.invited, emailCall{EventID: e.ID, AttendeeEmail: a.Email})
}

func (s *spyEmailSender) SendHostedEventUpdated(_ context.Context, e *models.HostedEvent, a *models.HostedEventAttendee, _ *models.Host, _ *models.Tenant, changed []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updated = append(s.updated, emailCall{EventID: e.ID, AttendeeEmail: a.Email, ChangedFields: append([]string(nil), changed...)})
}

func (s *spyEmailSender) SendHostedEventCancelled(_ context.Context, e *models.HostedEvent, a *models.HostedEventAttendee, _ *models.Host, _ *models.Tenant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelled = append(s.cancelled, emailCall{EventID: e.ID, AttendeeEmail: a.Email})
}

func (s *spyEmailSender) SendHostedEventCancelledForAttendee(_ context.Context, e *models.HostedEvent, a *models.HostedEventAttendee, _ *models.Host, _ *models.Tenant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelledForAttn = append(s.cancelledForAttn, emailCall{EventID: e.ID, AttendeeEmail: a.Email})
}

func (s *spyEmailSender) SendHostedEventReminder(_ context.Context, e *models.HostedEvent, a *models.HostedEventAttendee, _ *models.Host, _ *models.Tenant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reminders = append(s.reminders, emailCall{EventID: e.ID, AttendeeEmail: a.Email})
}

// invitedEmails returns the set of attendee emails that received an invite.
func (s *spyEmailSender) invitedEmails() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.invited))
	for i, c := range s.invited {
		out[i] = c.AttendeeEmail
	}
	return out
}

func (s *spyEmailSender) updatedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.updated)
}

func (s *spyEmailSender) cancelledForAttendeeEmails() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.cancelledForAttn))
	for i, c := range s.cancelledForAttn {
		out[i] = c.AttendeeEmail
	}
	return out
}

func (s *spyEmailSender) cancelledEmails() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.cancelled))
	for i, c := range s.cancelled {
		out[i] = c.AttendeeEmail
	}
	return out
}

// hostedEventHarness wires HostedEventService against real repos + a fake
// calendar writer + a spy email sender. Conferencing and contact run against
// real services; conferencing without a Zoom connection naturally returns
// ErrConferencingReauthRequired, which lets us cover the reauth path.
type hostedEventHarness struct {
	svc      *HostedEventService
	syncer   *CalendarEventSyncer
	repos    *repository.Repositories
	fake     *fakeCalendarWriter
	email    *spyEmailSender
	host     *models.Host
	tenant   *models.Tenant
	template *models.MeetingTemplate
	calID    string // primary writable calendar
	cleanup  func()
}

func makeHostedEventHarness(t *testing.T) *hostedEventHarness {
	t.Helper()
	_, repos, cleanup := setupTestRepos(t)
	ctx := context.Background()

	tenant := &models.Tenant{
		ID: uuid.New().String(), Slug: "tenant-" + uuid.New().String()[:6], Name: "Test Tenant",
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Tenant.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	host := &models.Host{
		ID: uuid.New().String(), TenantID: tenant.ID,
		Email: "h-" + uuid.New().String()[:4] + "@example.com", PasswordHash: "x",
		Name: "Test Host", Slug: "host-" + uuid.New().String()[:6],
		Timezone: "UTC", CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Host.Create(ctx, host); err != nil {
		t.Fatalf("create host: %v", err)
	}

	conn := &models.CalendarConnection{
		ID: uuid.New().String(), HostID: host.ID,
		Provider: models.CalendarProviderGoogle, Name: "primary-conn",
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Calendar.Create(ctx, conn); err != nil {
		t.Fatalf("create calendar connection: %v", err)
	}
	pc := &models.ProviderCalendar{
		ID: uuid.New().String(), ConnectionID: conn.ID,
		ProviderCalendarID: "primary", Name: "Primary", Color: "#000",
		IsPrimary: true, IsWritable: true, PollBusy: true,
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.ProviderCalendar.Create(ctx, pc); err != nil {
		t.Fatalf("create provider calendar: %v", err)
	}

	template := &models.MeetingTemplate{
		ID: uuid.New().String(), HostID: host.ID,
		Slug: "tmpl-" + uuid.New().String()[:6], Name: "Sales call",
		Description: "Default template", Durations: models.IntSlice{30},
		LocationType: models.ConferencingProviderGoogleMeet, CalendarID: pc.ID,
		MaxScheduleDays: 30, IsActive: true,
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Template.Create(ctx, template); err != nil {
		t.Fatalf("create template: %v", err)
	}

	cfg := &config.Config{}
	cfg.Server.BaseURL = "http://test.local"

	calendarSvc := NewCalendarService(cfg, repos)
	conferencingSvc := NewConferencingService(cfg, repos)
	contactSvc := NewContactService(repos)
	auditSvc := NewAuditLogService(repos)

	fake := &fakeCalendarWriter{}
	syncer := &CalendarEventSyncer{
		repos:    repos,
		calendar: fake,
		stores: map[ScheduledItemKind]trackingStore{
			ItemKindBooking:     &bookingTrackingStore{repo: repos.BookingCalendarEvent},
			ItemKindHostedEvent: &hostedEventTrackingStore{repo: repos.HostedEventCalendarEvent},
		},
	}

	emailSpy := &spyEmailSender{}
	svc := NewHostedEventService(cfg, repos, calendarSvc, conferencingSvc, syncer, emailSpy, contactSvc, auditSvc)

	return &hostedEventHarness{
		svc:      svc,
		syncer:   syncer,
		repos:    repos,
		fake:     fake,
		email:    emailSpy,
		host:     host,
		tenant:   tenant,
		template: template,
		calID:    pc.ID,
		cleanup:  cleanup,
	}
}

func (h *hostedEventHarness) baseCreateInput() CreateHostedEventInput {
	return CreateHostedEventInput{
		HostID:       h.host.ID,
		TenantID:     h.tenant.ID,
		Title:        "Quarterly review",
		Description:  "What we shipped, what's next.",
		Start:        time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		Duration:     30,
		Timezone:     "UTC",
		LocationType: models.ConferencingProviderGoogleMeet,
		CalendarID:   h.calID,
		Attendees: []AttendeeInput{
			{Email: "alice@example.com", Name: "Alice"},
			{Email: "bob@example.com", Name: "Bob"},
		},
	}
}

// waitForEmails spins until the spy reports the expected count, with a
// generous timeout to absorb the goroutines launched by EmailService-style
// senders. Our spy is synchronous, so this is effectively immediate, but the
// helper keeps tests robust against future async drift.
func (h *hostedEventHarness) waitForInvited(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(h.email.invitedEmails()) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %d invited emails, got %d", want, len(h.email.invitedEmails()))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHostedEventCreate_PersistsEventAndAttendees(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	out, err := h.svc.Create(ctx, h.baseCreateInput())
	if err != nil {
		t.Fatalf("Create returned err: %v", err)
	}
	if out == nil || out.Event == nil {
		t.Fatal("expected non-nil event")
	}
	if out.Event.Status != models.HostedEventStatusScheduled {
		t.Fatalf("status = %q, want scheduled", out.Event.Status)
	}
	if out.Event.EndTime.Sub(out.Event.StartTime.Time) != 30*time.Minute {
		t.Fatalf("end - start = %v, want 30m", out.Event.EndTime.Sub(out.Event.StartTime.Time))
	}
	persisted, err := h.repos.HostedEvent.GetByID(ctx, out.Event.ID)
	if err != nil || persisted == nil {
		t.Fatalf("persisted lookup: %v / %v", err, persisted)
	}

	attendees, err := h.repos.HostedEventAttendee.ListByEvent(ctx, out.Event.ID)
	if err != nil {
		t.Fatalf("ListByEvent: %v", err)
	}
	if len(attendees) != 2 {
		t.Fatalf("expected 2 attendee rows, got %d", len(attendees))
	}
	if attendees[0].Email != "alice@example.com" || attendees[1].Email != "bob@example.com" {
		t.Fatalf("attendee emails = %s, %s", attendees[0].Email, attendees[1].Email)
	}
}

func TestHostedEventCreate_ResolvesCalendarFromExplicit(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	out, err := h.svc.Create(ctx, h.baseCreateInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out.Event.CalendarID != h.calID {
		t.Fatalf("explicit calendar not preserved: got %q, want %q", out.Event.CalendarID, h.calID)
	}
}

func TestHostedEventCreate_ResolvesCalendarFromTemplateThenHost(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	// No explicit calendar. Template has CalendarID set; expect that.
	in := h.baseCreateInput()
	in.CalendarID = ""
	tmplID := h.template.ID
	in.TemplateID = &tmplID
	out, err := h.svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out.Event.CalendarID != h.template.CalendarID {
		t.Fatalf("template calendar not used: got %q, want %q", out.Event.CalendarID, h.template.CalendarID)
	}

	// No template either, but host has a default calendar.
	defaultCal := h.calID
	h.host.DefaultCalendarID = &defaultCal
	if err := h.repos.Host.Update(ctx, h.host); err != nil {
		t.Fatalf("update host: %v", err)
	}
	in2 := h.baseCreateInput()
	in2.CalendarID = ""
	in2.TemplateID = nil
	out2, err := h.svc.Create(ctx, in2)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out2.Event.CalendarID != defaultCal {
		t.Fatalf("host default calendar not used: got %q, want %q", out2.Event.CalendarID, defaultCal)
	}
}

func TestHostedEventCreate_ConferencingReauthRequired_StillPersistsEvent(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	// Zoom location type without any Zoom OAuth connection seeded.
	in := h.baseCreateInput()
	in.LocationType = models.ConferencingProviderZoom
	out, err := h.svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create returned err (should swallow reauth-required on create): %v", err)
	}
	if out == nil || out.Event == nil {
		t.Fatal("expected event to be persisted despite missing Zoom connection")
	}
	persisted, _ := h.repos.HostedEvent.GetByID(ctx, out.Event.ID)
	if persisted == nil {
		t.Fatal("event row missing after reauth-required path")
	}
	if persisted.ConferenceLink != "" {
		t.Fatalf("conference link should be empty when conferencing fails; got %q", persisted.ConferenceLink)
	}
}

func TestHostedEventCreate_GoogleMeet_LinkFromSyncerPersisted(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	// Simulate Google Meet returning a conference link via the calendar
	// create response (the second-write path).
	h.fake.nextCreate = []fakeCreateResult{{
		EventID: "calendar-evt-1", ConferenceLink: "https://meet.google.com/zzz-aaa-bbb",
	}}

	out, err := h.svc.Create(ctx, h.baseCreateInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	persisted, _ := h.repos.HostedEvent.GetByID(ctx, out.Event.ID)
	if persisted == nil {
		t.Fatal("event missing")
	}
	if persisted.ConferenceLink != "https://meet.google.com/zzz-aaa-bbb" {
		t.Fatalf("syncer-returned conference link not persisted: got %q", persisted.ConferenceLink)
	}
}

func TestHostedEventCreate_SendsInvitedEmailToEachAttendee(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	if _, err := h.svc.Create(ctx, h.baseCreateInput()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	h.waitForInvited(t, 2)
	got := h.email.invitedEmails()
	if !contains(got, "alice@example.com") || !contains(got, "bob@example.com") {
		t.Fatalf("invited recipients = %v, missing one of alice/bob", got)
	}
}

func TestHostedEventCreate_UpsertsContactsForAttendees(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	if _, err := h.svc.Create(ctx, h.baseCreateInput()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Contact upsert is fire-and-forget but synchronous to the repo write
	// inside ContactService.UpsertFromHostedEventAttendee. Allow a small
	// settle window.
	time.Sleep(20 * time.Millisecond)

	c1, err := h.repos.Contact.GetByEmail(ctx, h.tenant.ID, "alice@example.com")
	if err != nil {
		t.Fatalf("get contact alice: %v", err)
	}
	if c1 == nil {
		t.Fatal("expected contact for alice@example.com to be upserted")
	}
}

func TestHostedEventUpdate_AttendeeDiff_RoutesEmailsCorrectly(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	created, err := h.svc.Create(ctx, h.baseCreateInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	h.waitForInvited(t, 2)

	// Reset the spy so we only see the update-time emails.
	h.email = &spyEmailSender{}
	h.svc.email = h.email

	// Bob removed, Carol added, Alice retained.
	newAttendees := []AttendeeInput{
		{Email: "alice@example.com", Name: "Alice"},
		{Email: "carol@example.com", Name: "Carol"},
	}
	newTitle := "Quarterly review (updated)"
	if _, _, err := h.svc.Update(ctx, UpdateHostedEventInput{
		HostID:    h.host.ID,
		TenantID:  h.tenant.ID,
		EventID:   created.Event.ID,
		Title:     &newTitle,
		Attendees: &newAttendees,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Expect: 1 invited (Carol), 1 cancelled-for-attendee (Bob),
	// 1 updated (Alice retained, title changed).
	time.Sleep(30 * time.Millisecond)

	if got := h.email.invitedEmails(); !equalSet(got, []string{"carol@example.com"}) {
		t.Fatalf("invited = %v, want [carol]", got)
	}
	if got := h.email.cancelledForAttendeeEmails(); !equalSet(got, []string{"bob@example.com"}) {
		t.Fatalf("cancelled-for-attendee = %v, want [bob]", got)
	}
	if h.email.updatedCount() != 1 {
		t.Fatalf("expected 1 updated email (alice retained), got %d", h.email.updatedCount())
	}
}

func TestHostedEventUpdate_TimeChange_ResetsReminderSent(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	created, err := h.svc.Create(ctx, h.baseCreateInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Mark reminder as already sent.
	if err := h.repos.HostedEvent.MarkReminderSent(ctx, created.Event.ID); err != nil {
		t.Fatalf("MarkReminderSent: %v", err)
	}
	pre, _ := h.repos.HostedEvent.GetByID(ctx, created.Event.ID)
	if !pre.ReminderSent {
		t.Fatal("precondition: reminder_sent should be true after MarkReminderSent")
	}

	newStart := created.Event.StartTime.Add(2 * time.Hour)
	if _, _, err := h.svc.Update(ctx, UpdateHostedEventInput{
		HostID:   h.host.ID,
		TenantID: h.tenant.ID,
		EventID:  created.Event.ID,
		Start:    &newStart,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	post, _ := h.repos.HostedEvent.GetByID(ctx, created.Event.ID)
	if post.ReminderSent {
		t.Fatal("reminder_sent should reset to false after Start change")
	}
}

func TestHostedEventUpdate_NoMaterialChange_SendsNoEmail(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	created, err := h.svc.Create(ctx, h.baseCreateInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	h.waitForInvited(t, 2)
	h.email = &spyEmailSender{}
	h.svc.email = h.email

	// Update with no actual changes.
	sameTitle := created.Event.Title
	if _, changed, err := h.svc.Update(ctx, UpdateHostedEventInput{
		HostID:   h.host.ID,
		TenantID: h.tenant.ID,
		EventID:  created.Event.ID,
		Title:    &sameTitle,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	} else if len(changed) != 0 {
		t.Fatalf("expected no changed fields, got %v", changed)
	}

	time.Sleep(20 * time.Millisecond)
	if h.email.updatedCount() != 0 || len(h.email.invitedEmails()) != 0 {
		t.Fatalf("no-op update should send no email; updated=%d invited=%d",
			h.email.updatedCount(), len(h.email.invitedEmails()))
	}
}

func TestHostedEventUpdate_PatchesAllTrackedCalendarEvents(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	created, err := h.svc.Create(ctx, h.baseCreateInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// One tracking row for the owner.
	rows, _ := h.repos.HostedEventCalendarEvent.GetByHostedEventID(ctx, created.Event.ID)
	if len(rows) != 1 {
		t.Fatalf("expected 1 tracking row after create, got %d", len(rows))
	}
	h.fake.updateCalls = nil

	newTitle := "Renamed"
	if _, _, err := h.svc.Update(ctx, UpdateHostedEventInput{
		HostID: h.host.ID, TenantID: h.tenant.ID, EventID: created.Event.ID,
		Title: &newTitle,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := len(h.fake.updateCalls); got != 1 {
		t.Fatalf("expected 1 calendar update call, got %d", got)
	}
}

func TestHostedEventUpdate_ContactNotReupsertedForRetained(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	created, err := h.svc.Create(ctx, h.baseCreateInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	preAlice, _ := h.repos.Contact.GetByEmail(ctx, h.tenant.ID, "alice@example.com")
	if preAlice == nil {
		t.Fatal("alice contact missing after create")
	}
	preCount := preAlice.MeetingCount

	// Update title only, keep both attendees. No new contact upsert should
	// run for either retained attendee.
	newTitle := "Renamed"
	sameAttendees := []AttendeeInput{
		{Email: "alice@example.com", Name: "Alice"},
		{Email: "bob@example.com", Name: "Bob"},
	}
	if _, _, err := h.svc.Update(ctx, UpdateHostedEventInput{
		HostID: h.host.ID, TenantID: h.tenant.ID, EventID: created.Event.ID,
		Title: &newTitle, Attendees: &sameAttendees,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	postAlice, _ := h.repos.Contact.GetByEmail(ctx, h.tenant.ID, "alice@example.com")
	if postAlice == nil {
		t.Fatal("alice contact disappeared")
	}
	if postAlice.MeetingCount != preCount {
		t.Fatalf("retained attendee meeting_count was re-incremented: %d → %d", preCount, postAlice.MeetingCount)
	}
}

func TestHostedEventCancel_DeletesTrackedRowsAndEmails(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	created, err := h.svc.Create(ctx, h.baseCreateInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	h.waitForInvited(t, 2)

	if err := h.svc.Cancel(ctx, h.host.ID, h.tenant.ID, created.Event.ID, "no longer needed"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	rows, _ := h.repos.HostedEventCalendarEvent.GetByHostedEventID(ctx, created.Event.ID)
	if len(rows) != 0 {
		t.Fatalf("expected 0 tracking rows after cancel, got %d", len(rows))
	}
	persisted, _ := h.repos.HostedEvent.GetByID(ctx, created.Event.ID)
	if persisted.Status != models.HostedEventStatusCancelled {
		t.Fatalf("status = %q, want cancelled", persisted.Status)
	}
	if persisted.CancelReason != "no longer needed" {
		t.Fatalf("cancel reason = %q", persisted.CancelReason)
	}
	time.Sleep(20 * time.Millisecond)
	if got := h.email.cancelledEmails(); !equalSet(got, []string{"alice@example.com", "bob@example.com"}) {
		t.Fatalf("cancelled emails = %v, want [alice, bob]", got)
	}
}

func TestHostedEventDetectBusyConflicts_ExcludesSelf(t *testing.T) {
	h := makeHostedEventHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	created, err := h.svc.Create(ctx, h.baseCreateInput())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Same window, excluding the event itself → no hosted-event conflict.
	start := created.Event.StartTime.Time
	end := created.Event.EndTime.Time
	conflicts, err := h.svc.DetectBusyConflicts(ctx, h.host.ID, start, end, created.Event.ID)
	if err != nil {
		t.Fatalf("DetectBusyConflicts: %v", err)
	}
	for _, c := range conflicts {
		if c.Start.Equal(start) && c.End.Equal(end) {
			t.Fatalf("self-event reported as conflict; should be excluded")
		}
	}

	// Without exclusion, the event itself shows up.
	conflicts2, err := h.svc.DetectBusyConflicts(ctx, h.host.ID, start, end, "")
	if err != nil {
		t.Fatalf("DetectBusyConflicts (no exclude): %v", err)
	}
	hit := false
	for _, c := range conflicts2 {
		if c.Start.Equal(start) && c.End.Equal(end) {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("expected hosted event to surface in own-window conflict scan, got none")
	}
}

// ---------------------------------------------------------------------------
// Small assertion helpers
// ---------------------------------------------------------------------------

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func equalSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	mb := make(map[string]int, len(b))
	for _, v := range b {
		mb[v]++
	}
	for _, v := range a {
		if mb[v] == 0 {
			return false
		}
		mb[v]--
	}
	return true
}
