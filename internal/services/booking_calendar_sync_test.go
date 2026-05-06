package services

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// fakeCalendarWriter records calls and returns programmed responses. Lets
// syncer tests run without hitting the network and lets us assert exact
// per-host fan-out behaviour.
type fakeCalendarWriter struct {
	createCalls []fakeCreateCall
	updateCalls []fakeUpdateCall
	deleteCalls []fakeDeleteCall

	// nextCreate is consulted in order; missing entries default to a generated
	// success.
	nextCreate []fakeCreateResult
	// nextUpdate likewise.
	nextUpdate []fakeUpdateResult

	// createErrForCalendarID: any create call whose target calendar matches
	// returns the configured error.
	createErrForCalendarID map[string]error
	updateErrForCalendarID map[string]error
}

type fakeCreateCall struct {
	CalendarID string
	Input      CalendarEventInput
}

type fakeUpdateCall struct {
	CalendarID string
	Input      CalendarEventInput
}

type fakeDeleteCall struct {
	HostID, CalendarID, EventID string
}

type fakeCreateResult struct {
	EventID        string
	ConferenceLink string
	Err            error
}

type fakeUpdateResult struct {
	EventID string
	Err     error
}

func (f *fakeCalendarWriter) CreateEventForHost(_ context.Context, providerCalendarID string, input *CalendarEventInput) (string, string, error) {
	f.createCalls = append(f.createCalls, fakeCreateCall{CalendarID: providerCalendarID, Input: *input})
	if err, ok := f.createErrForCalendarID[providerCalendarID]; ok {
		return "", "", err
	}
	if len(f.nextCreate) > 0 {
		r := f.nextCreate[0]
		f.nextCreate = f.nextCreate[1:]
		return r.EventID, r.ConferenceLink, r.Err
	}
	return "evt-" + providerCalendarID + "-" + uuid.New().String()[:8], "", nil
}

func (f *fakeCalendarWriter) UpdateEvent(_ context.Context, providerCalendarID string, input *CalendarEventInput) (string, error) {
	f.updateCalls = append(f.updateCalls, fakeUpdateCall{CalendarID: providerCalendarID, Input: *input})
	if err, ok := f.updateErrForCalendarID[providerCalendarID]; ok {
		return "", err
	}
	if len(f.nextUpdate) > 0 {
		r := f.nextUpdate[0]
		f.nextUpdate = f.nextUpdate[1:]
		return r.EventID, r.Err
	}
	return input.EventID, nil
}

func (f *fakeCalendarWriter) DeleteEvent(_ context.Context, hostID, providerCalendarID, eventID string) error {
	f.deleteCalls = append(f.deleteCalls, fakeDeleteCall{HostID: hostID, CalendarID: providerCalendarID, EventID: eventID})
	return nil
}

// fixture holds the minimum row set needed to satisfy booking_calendar_events
// foreign keys.
type fixture struct {
	tenantID   string
	templateID string
	bookingID  string
	hostIDs    []string
	calendars  map[string]string // hostID → provider_calendar.id
}

// syncerHarness bundles a syncer wired to real repositories with the fake
// calendar writer and a seeded fixture.
type syncerHarness struct {
	syncer  *CalendarEventSyncer
	fake    *fakeCalendarWriter
	fixture *fixture
	cleanup func()
}

func makeSyncerHarness(t *testing.T, hostCount int) *syncerHarness {
	t.Helper()
	db, repos, cleanup := setupTestRepos(t)
	fix := seedFixture(t, db, repos, hostCount)
	fake := &fakeCalendarWriter{}
	s := &CalendarEventSyncer{
		repos:    repos,
		calendar: fake,
		stores: map[ScheduledItemKind]trackingStore{
			ItemKindBooking: &bookingTrackingStore{repo: repos.BookingCalendarEvent},
		},
	}
	return &syncerHarness{syncer: s, fake: fake, fixture: fix, cleanup: cleanup}
}

func seedFixture(t *testing.T, db *sql.DB, repos *repositoryWrapper, hostCount int) *fixture {
	t.Helper()
	ctx := context.Background()
	short := func(s string) string { return s[:8] }

	tenant := &models.Tenant{
		ID: uuid.New().String(), Slug: "t-" + uuid.New().String()[:6], Name: "Test Tenant",
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Tenant.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	hosts := make([]string, 0, hostCount)
	cals := map[string]string{}
	for i := 0; i < hostCount; i++ {
		host := &models.Host{
			ID:           uuid.New().String(),
			TenantID:     tenant.ID,
			Email:        "h" + uuid.New().String()[:4] + "@example.com",
			PasswordHash: "x",
			Name:         "Host",
			Slug:         "host-" + uuid.New().String()[:6],
			Timezone:     "UTC",
			CreatedAt:    models.Now(),
			UpdatedAt:    models.Now(),
		}
		if err := repos.Host.Create(ctx, host); err != nil {
			t.Fatalf("create host: %v", err)
		}
		hosts = append(hosts, host.ID)

		conn := &models.CalendarConnection{
			ID: uuid.New().String(), HostID: host.ID,
			Provider: models.CalendarProviderGoogle, Name: "gmail",
			CreatedAt: models.Now(), UpdatedAt: models.Now(),
		}
		if err := repos.Calendar.Create(ctx, conn); err != nil {
			t.Fatalf("create calendar conn: %v", err)
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
		cals[host.ID] = pc.ID
	}

	owner := hosts[0]
	tmpl := &models.MeetingTemplate{
		ID: uuid.New().String(), HostID: owner,
		Slug: "test-" + short(uuid.New().String()), Name: "Test Template",
		Durations: models.IntSlice{30}, LocationType: models.ConferencingProviderGoogleMeet,
		CalendarID: cals[owner], MaxScheduleDays: 30, IsActive: true,
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Template.Create(ctx, tmpl); err != nil {
		t.Fatalf("create template: %v", err)
	}

	bk := &models.Booking{
		ID: uuid.New().String(), TemplateID: tmpl.ID, HostID: owner,
		Token: uuid.New().String(), Status: models.BookingStatusConfirmed,
		StartTime: models.Now(), EndTime: models.Now(), Duration: 30,
		InviteeName: "Inv", InviteeEmail: "i@example.com",
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Booking.Create(ctx, bk); err != nil {
		t.Fatalf("create booking: %v", err)
	}

	return &fixture{
		tenantID:   tenant.ID,
		templateID: tmpl.ID,
		bookingID:  bk.ID,
		hostIDs:    hosts,
		calendars:  cals,
	}
}

// repositoryWrapper is just an alias to keep the seed signature compact.
type repositoryWrapper = repository.Repositories

func basicInput() CalendarEventInput {
	return CalendarEventInput{
		Summary:     "Sync test",
		Description: "Body",
		Start:       time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC),
		End:         time.Date(2026, 1, 2, 11, 0, 0, 0, time.UTC),
		Attendees:   []string{"a@example.com"},
		HostName:    "Host",
		HostEmail:   "host@example.com",
	}
}

func ownerTarget(fix *fixture) HostTarget {
	return HostTarget{HostID: fix.hostIDs[0], CalendarID: fix.calendars[fix.hostIDs[0]], IsOwner: true}
}

func TestSyncerCreate_OneHost_WritesEventAndTrackingRow(t *testing.T) {
	h := makeSyncerHarness(t, 1)
	defer h.cleanup()
	ctx := context.Background()

	firstID, link, err := h.syncer.Create(ctx, CalendarSyncRequest{
		Kind:   ItemKindBooking,
		ItemID: h.fixture.bookingID,
		Input:  basicInput(),
		Hosts:  []HostTarget{ownerTarget(h.fixture)},
	})
	if err != nil {
		t.Fatalf("Create returned err: %v", err)
	}
	if firstID == "" {
		t.Fatal("expected non-empty first event ID")
	}
	if link != "" {
		t.Fatalf("expected empty conference link, got %q", link)
	}
	if got := len(h.fake.createCalls); got != 1 {
		t.Fatalf("expected 1 fake create call, got %d", got)
	}
	rows, err := h.syncer.repos.BookingCalendarEvent.GetByBookingID(ctx, h.fixture.bookingID)
	if err != nil {
		t.Fatalf("GetByBookingID: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 tracking row, got %d", len(rows))
	}
	if rows[0].EventID != firstID {
		t.Fatalf("tracking row event_id = %q, want %q", rows[0].EventID, firstID)
	}
}

func TestSyncerCreate_PooledHosts_OneEventPerHost(t *testing.T) {
	h := makeSyncerHarness(t, 3)
	defer h.cleanup()
	ctx := context.Background()
	fix := h.fixture

	hosts := []HostTarget{
		{HostID: fix.hostIDs[0], CalendarID: fix.calendars[fix.hostIDs[0]], IsOwner: true},
		{HostID: fix.hostIDs[1], CalendarID: fix.calendars[fix.hostIDs[1]]},
		{HostID: fix.hostIDs[2], CalendarID: fix.calendars[fix.hostIDs[2]]},
	}
	firstID, _, err := h.syncer.Create(ctx, CalendarSyncRequest{
		Kind:   ItemKindBooking,
		ItemID: fix.bookingID,
		Input:  basicInput(),
		Hosts:  hosts,
	})
	if err != nil {
		t.Fatalf("Create returned err: %v", err)
	}
	if got := len(h.fake.createCalls); got != 3 {
		t.Fatalf("expected 3 fake create calls, got %d", got)
	}
	rows, _ := h.syncer.repos.BookingCalendarEvent.GetByBookingID(ctx, fix.bookingID)
	if len(rows) != 3 {
		t.Fatalf("expected 3 tracking rows, got %d", len(rows))
	}
	var ownerRow *models.BookingCalendarEvent
	for _, r := range rows {
		if r.HostID == fix.hostIDs[0] {
			ownerRow = r
			break
		}
	}
	if ownerRow == nil {
		t.Fatal("owner tracking row missing")
	}
	if firstID != ownerRow.EventID {
		t.Fatalf("first event ID %q does not match owner tracking row %q", firstID, ownerRow.EventID)
	}
}

func TestSyncerCreate_PerHostFailureContinues(t *testing.T) {
	h := makeSyncerHarness(t, 3)
	defer h.cleanup()
	fix := h.fixture
	failedCal := fix.calendars[fix.hostIDs[1]]
	h.fake.createErrForCalendarID = map[string]error{failedCal: errors.New("boom")}

	ctx := context.Background()
	hosts := []HostTarget{
		{HostID: fix.hostIDs[0], CalendarID: fix.calendars[fix.hostIDs[0]], IsOwner: true},
		{HostID: fix.hostIDs[1], CalendarID: fix.calendars[fix.hostIDs[1]]},
		{HostID: fix.hostIDs[2], CalendarID: fix.calendars[fix.hostIDs[2]]},
	}
	if _, _, err := h.syncer.Create(ctx, CalendarSyncRequest{
		Kind:   ItemKindBooking,
		ItemID: fix.bookingID,
		Input:  basicInput(),
		Hosts:  hosts,
	}); err != nil {
		t.Fatalf("Create returned err: %v", err)
	}
	rows, _ := h.syncer.repos.BookingCalendarEvent.GetByBookingID(ctx, fix.bookingID)
	if len(rows) != 2 {
		t.Fatalf("expected 2 tracking rows (failed host skipped), got %d", len(rows))
	}
	for _, r := range rows {
		if r.HostID == fix.hostIDs[1] {
			t.Fatal("failed host should not have a tracking row")
		}
	}
}

func TestSyncerCreate_GoogleMeet_ReturnsConferenceLink(t *testing.T) {
	h := makeSyncerHarness(t, 1)
	defer h.cleanup()
	h.fake.nextCreate = []fakeCreateResult{
		{EventID: "g-evt-1", ConferenceLink: "https://meet.google.com/xyz-abc-def"},
	}

	_, link, err := h.syncer.Create(context.Background(), CalendarSyncRequest{
		Kind:   ItemKindBooking,
		ItemID: h.fixture.bookingID,
		Input:  basicInput(),
		Hosts:  []HostTarget{ownerTarget(h.fixture)},
	})
	if err != nil {
		t.Fatalf("Create err: %v", err)
	}
	if link != "https://meet.google.com/xyz-abc-def" {
		t.Fatalf("unexpected conference link: %q", link)
	}
}

func TestSyncerUpdate_PatchesAllTrackedEvents(t *testing.T) {
	h := makeSyncerHarness(t, 2)
	defer h.cleanup()
	ctx := context.Background()
	fix := h.fixture

	hosts := []HostTarget{
		{HostID: fix.hostIDs[0], CalendarID: fix.calendars[fix.hostIDs[0]], IsOwner: true},
		{HostID: fix.hostIDs[1], CalendarID: fix.calendars[fix.hostIDs[1]]},
	}
	if _, _, err := h.syncer.Create(ctx, CalendarSyncRequest{
		Kind: ItemKindBooking, ItemID: fix.bookingID, Input: basicInput(), Hosts: hosts,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	h.fake.updateCalls = nil

	if err := h.syncer.Update(ctx, CalendarSyncRequest{
		Kind: ItemKindBooking, ItemID: fix.bookingID, Input: basicInput(),
	}); err != nil {
		t.Fatalf("Update err: %v", err)
	}
	if got := len(h.fake.updateCalls); got != 2 {
		t.Fatalf("expected 2 update calls, got %d", got)
	}
}

// Replacement event_id from CalDAV-style delete+create path is persisted
// back to the tracking row.
func TestSyncerUpdate_CalDAVNewEventID_PersistsToTrackingRow(t *testing.T) {
	h := makeSyncerHarness(t, 1)
	defer h.cleanup()
	ctx := context.Background()
	fix := h.fixture

	if _, _, err := h.syncer.Create(ctx, CalendarSyncRequest{
		Kind: ItemKindBooking, ItemID: fix.bookingID, Input: basicInput(),
		Hosts: []HostTarget{ownerTarget(fix)},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	h.fake.nextUpdate = []fakeUpdateResult{{EventID: "fresh-id-from-caldav"}}

	if err := h.syncer.Update(ctx, CalendarSyncRequest{
		Kind: ItemKindBooking, ItemID: fix.bookingID, Input: basicInput(),
	}); err != nil {
		t.Fatalf("Update err: %v", err)
	}
	rows, _ := h.syncer.repos.BookingCalendarEvent.GetByBookingID(ctx, fix.bookingID)
	if len(rows) != 1 {
		t.Fatalf("expected 1 tracking row, got %d", len(rows))
	}
	if rows[0].EventID != "fresh-id-from-caldav" {
		t.Fatalf("tracking row was not updated to the replacement ID; got %q", rows[0].EventID)
	}
}

// When the prior event_id is empty (an earlier create failed and the row was
// seeded with EventID=""), a subsequent Update must persist whatever new ID
// the calendar writer returns.
func TestSyncerUpdate_PriorIDEmpty_PersistsCreatedID(t *testing.T) {
	h := makeSyncerHarness(t, 1)
	defer h.cleanup()
	ctx := context.Background()
	fix := h.fixture
	owner := fix.hostIDs[0]

	// Seed a tracking row with EventID="".
	if err := h.syncer.repos.BookingCalendarEvent.Create(ctx, &models.BookingCalendarEvent{
		ID:         uuid.New().String(),
		BookingID:  fix.bookingID,
		HostID:     owner,
		CalendarID: fix.calendars[owner],
		EventID:    "",
		CreatedAt:  models.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	h.fake.nextUpdate = []fakeUpdateResult{{EventID: "first-create-id"}}

	if err := h.syncer.Update(ctx, CalendarSyncRequest{
		Kind: ItemKindBooking, ItemID: fix.bookingID, Input: basicInput(),
	}); err != nil {
		t.Fatalf("Update err: %v", err)
	}
	rows, _ := h.syncer.repos.BookingCalendarEvent.GetByBookingID(ctx, fix.bookingID)
	if len(rows) != 1 {
		t.Fatalf("expected 1 tracking row, got %d", len(rows))
	}
	if rows[0].EventID != "first-create-id" {
		t.Fatalf("tracking row event_id = %q, want first-create-id", rows[0].EventID)
	}
}

func TestSyncerDelete_RemovesAllTrackedRows(t *testing.T) {
	h := makeSyncerHarness(t, 2)
	defer h.cleanup()
	ctx := context.Background()
	fix := h.fixture

	hosts := []HostTarget{
		{HostID: fix.hostIDs[0], CalendarID: fix.calendars[fix.hostIDs[0]], IsOwner: true},
		{HostID: fix.hostIDs[1], CalendarID: fix.calendars[fix.hostIDs[1]]},
	}
	if _, _, err := h.syncer.Create(ctx, CalendarSyncRequest{
		Kind: ItemKindBooking, ItemID: fix.bookingID, Input: basicInput(), Hosts: hosts,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	h.fake.deleteCalls = nil

	count, err := h.syncer.Delete(ctx, ItemKindBooking, fix.bookingID)
	if err != nil {
		t.Fatalf("Delete err: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count=2, got %d", count)
	}
	if got := len(h.fake.deleteCalls); got != 2 {
		t.Fatalf("expected 2 delete calls, got %d", got)
	}
	rows, _ := h.syncer.repos.BookingCalendarEvent.GetByBookingID(ctx, fix.bookingID)
	if len(rows) != 0 {
		t.Fatalf("expected 0 tracking rows after delete, got %d", len(rows))
	}
}

func TestSyncerDelete_NoTrackedRows_ReturnsZero(t *testing.T) {
	h := makeSyncerHarness(t, 1)
	defer h.cleanup()
	count, err := h.syncer.Delete(context.Background(), ItemKindBooking, "no-such-booking")
	if err != nil {
		t.Fatalf("Delete err: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count=0, got %d", count)
	}
}
