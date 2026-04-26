package services

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/database"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// TestAssignProviderCalendarColors_StableAndUnique mirrors the connection-level
// AssignColors test for the new ProviderCalendar variant.
func TestAssignProviderCalendarColors_StableAndUnique(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cals := []*models.ProviderCalendar{
		{ID: "c", CreatedAt: models.NewSQLiteTime(base.Add(2 * time.Minute))},
		{ID: "a", CreatedAt: models.NewSQLiteTime(base)},
		{ID: "b", CreatedAt: models.NewSQLiteTime(base.Add(time.Minute))},
	}
	AssignProviderCalendarColors(cals)

	seen := map[string]bool{}
	for _, c := range cals {
		if c.Color == "" {
			t.Errorf("calendar %s ended up with empty color", c.ID)
		}
		if seen[c.Color] {
			t.Errorf("duplicate color %s assigned", c.Color)
		}
		seen[c.Color] = true
	}

	// The oldest by CreatedAt (id "a") should get CalendarPalette[0] regardless
	// of input order.
	for _, c := range cals {
		if c.ID != "a" {
			continue
		}
		if c.Color != CalendarPalette[0] {
			t.Errorf("oldest calendar: want %s, got %s", CalendarPalette[0], c.Color)
		}
	}
}

// TestNormalizeColor checks that we reject malformed hex values (so we don't
// import invalid strings from the provider) and accept the canonical form.
func TestNormalizeColor(t *testing.T) {
	cases := map[string]string{
		"#378ADD":  "#378ADD",
		"#378add":  "#378ADD",
		"":         "",
		"red":      "",
		"#ABC":     "",
		"#ABCDEFG": "",
	}
	for in, want := range cases {
		if got := normalizeColor(in); got != want {
			t.Errorf("normalizeColor(%q) = %q, want %q", in, got, want)
		}
	}
}

// setupServiceTestDB creates a fresh test DB with all migrations applied and
// returns the wired repositories + a CalendarService bound to a test config.
func setupServiceTestDB(t *testing.T) (*sql.DB, *repository.Repositories, *CalendarService) {
	t.Helper()
	cfg := &config.Config{}
	cfg.Server.BaseURL = "http://test"
	dbCfg := config.DatabaseConfig{
		Driver:         "sqlite",
		Name:           ":memory:",
		MigrationsPath: "../../migrations",
	}
	db, err := database.New(dbCfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(db, dbCfg); err != nil {
		db.Close()
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.NewRepositories(db, dbCfg.Driver)
	cal := NewCalendarService(cfg, repos)
	return db, repos, cal
}

// seedHostAndConnection writes the minimal set of rows the tests need.
func seedHostAndConnection(t *testing.T, repos *repository.Repositories, provider models.CalendarProvider, accessToken, caldavURL string) (*models.Host, *models.CalendarConnection) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.New().String()[:8]
	tenant := &models.Tenant{
		ID: uuid.New().String(), Slug: "t-" + suffix, Name: "T",
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Tenant.Create(ctx, tenant); err != nil {
		t.Fatalf("tenant: %v", err)
	}
	host := &models.Host{
		ID: uuid.New().String(), TenantID: tenant.ID,
		Email: "h-" + suffix + "@example.com", PasswordHash: "x",
		Name: "Host", Slug: "h-" + suffix, Timezone: "UTC",
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Host.Create(ctx, host); err != nil {
		t.Fatalf("host: %v", err)
	}
	expiry := models.NewSQLiteTime(time.Now().Add(time.Hour))
	conn := &models.CalendarConnection{
		ID: uuid.New().String(), HostID: host.ID, Provider: provider,
		Name: "Account", AccessToken: accessToken, RefreshToken: "rt",
		TokenExpiry: &expiry, CalDAVURL: caldavURL, CalDAVUsername: "u", CalDAVPassword: "p",
		SyncStatus: models.CalendarSyncStatusUnknown,
		CreatedAt:  models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Calendar.Create(ctx, conn); err != nil {
		t.Fatalf("conn: %v", err)
	}
	return host, conn
}

// TestGetBusyTimes_SkipsCalendarsWithPollBusyFalse is the critical correctness
// test for the new feature: a calendar whose user has toggled poll_busy off
// must NOT have its busy times included in availability.
func TestGetBusyTimes_SkipsCalendarsWithPollBusyFalse(t *testing.T) {
	// Mock Google freeBusy server. We assert that only the polled calendar id
	// appears in the request and that the busy slot it returns is included.
	var seenItems []string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readReqBody(r)
		// Crude but sufficient: the freeBusy request body is JSON and we just
		// look for the calendar IDs in the items array.
		for _, candidate := range []string{"work@example.com", "holidays@example.com"} {
			if strings.Contains(body, `"id":"`+candidate+`"`) {
				seenItems = append(seenItems, candidate)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"calendars":{"work@example.com":{"busy":[{"start":"2026-01-01T10:00:00Z","end":"2026-01-01T11:00:00Z"}]}}}`))
	}))
	defer mock.Close()

	// Re-route Google's freebusy endpoint to the mock. The CalendarService uses
	// the literal URL from the source, so we patch it with a temporary
	// transport. Easier: temporarily install a httpmock-like roundtripper.
	origTransport := http.DefaultTransport
	http.DefaultTransport = &mockTransport{base: origTransport, redirect: map[string]string{
		"https://www.googleapis.com/calendar/v3/freeBusy": mock.URL + "/freeBusy",
	}}
	defer func() { http.DefaultTransport = origTransport }()
	http.DefaultClient = &http.Client{Transport: http.DefaultTransport}
	defer func() { http.DefaultClient = &http.Client{} }()

	_, repos, cal := setupServiceTestDB(t)
	host, conn := seedHostAndConnection(t, repos, models.CalendarProviderGoogle, "tok", "")

	ctx := context.Background()
	pcWork, _ := repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, "work@example.com", "Work", "", true, true)
	pcHolidays, _ := repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, "holidays@example.com", "Holidays", "", false, true)

	_ = pcWork
	// User disables polling for holidays.
	if err := repos.ProviderCalendar.UpdatePollBusy(ctx, host.ID, pcHolidays.ID, false); err != nil {
		t.Fatalf("disable poll: %v", err)
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	slots, err := cal.GetBusyTimes(ctx, host.ID, start, end)
	if err != nil {
		t.Fatalf("GetBusyTimes: %v", err)
	}
	if len(slots) != 1 {
		t.Fatalf("expected 1 busy slot from work calendar, got %d", len(slots))
	}

	// Holidays must NOT have appeared in the freeBusy request.
	for _, id := range seenItems {
		if id == "holidays@example.com" {
			t.Errorf("holidays calendar was queried despite poll_busy=false")
		}
	}
}

// TestGetBusyTimes_GroupsCallsByConnection verifies that two calendars under
// the same connection cause one freeBusy request, not two.
func TestGetBusyTimes_GroupsCallsByConnection(t *testing.T) {
	calls := 0
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"calendars":{}}`))
	}))
	defer mock.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = &mockTransport{base: origTransport, redirect: map[string]string{
		"https://www.googleapis.com/calendar/v3/freeBusy": mock.URL + "/freeBusy",
	}}
	defer func() { http.DefaultTransport = origTransport }()
	http.DefaultClient = &http.Client{Transport: http.DefaultTransport}
	defer func() { http.DefaultClient = &http.Client{} }()

	_, repos, cal := setupServiceTestDB(t)
	host, conn := seedHostAndConnection(t, repos, models.CalendarProviderGoogle, "tok", "")

	ctx := context.Background()
	for _, name := range []string{"a", "b", "c"} {
		_, _ = repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, name+"@example.com", name, "", false, true)
	}

	start := time.Now()
	if _, err := cal.GetBusyTimes(ctx, host.ID, start, start.Add(time.Hour)); err != nil {
		t.Fatalf("GetBusyTimes: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 freeBusy call (calendars under one connection), got %d", calls)
	}
}

// TestRefreshGoogleCalendarList_UpsertsAndDeletesMissing verifies the
// list-sync behaviour: existing calendars get their metadata refreshed, new
// calendars are created, and calendars that disappear from Google are removed.
func TestRefreshGoogleCalendarList_UpsertsAndDeletesMissing(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{"id":"work@example.com","summary":"Work","backgroundColor":"#378ADD","primary":true,"accessRole":"owner"},
			{"id":"shared@example.com","summary":"Shared","backgroundColor":"#1D9E75","accessRole":"reader"}
		]}`))
	}))
	defer mock.Close()

	origTransport := http.DefaultTransport
	http.DefaultTransport = &mockTransport{base: origTransport, redirect: map[string]string{
		"https://www.googleapis.com/calendar/v3/users/me/calendarList?maxResults=250": mock.URL + "/list",
	}}
	defer func() { http.DefaultTransport = origTransport }()
	http.DefaultClient = &http.Client{Transport: http.DefaultTransport}
	defer func() { http.DefaultClient = &http.Client{} }()

	_, repos, cal := setupServiceTestDB(t)
	_, conn := seedHostAndConnection(t, repos, models.CalendarProviderGoogle, "tok", "")

	ctx := context.Background()

	// Pre-seed an "old" calendar that should be deleted by the refresh.
	_, _ = repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, "deleted@example.com", "Deleted", "", false, true)

	saved, err := cal.refreshGoogleCalendarList(ctx, conn)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("expected 2 saved, got %d", len(saved))
	}

	all, err := repos.ProviderCalendar.GetByConnectionID(ctx, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]*models.ProviderCalendar{}
	for _, pc := range all {
		got[pc.ProviderCalendarID] = pc
	}
	if got["deleted@example.com"] != nil {
		t.Error("expected deleted@example.com to be removed")
	}
	if got["work@example.com"] == nil || !got["work@example.com"].IsPrimary || !got["work@example.com"].IsWritable {
		t.Errorf("work@example.com missing or with wrong flags: %+v", got["work@example.com"])
	}
	if got["shared@example.com"] == nil {
		t.Fatal("expected shared@example.com in result")
	}
	if got["shared@example.com"].IsWritable {
		t.Error("reader-role calendar must be marked is_writable=false")
	}
}

// mockTransport rewrites outbound URLs that match its redirect map.
type mockTransport struct {
	base     http.RoundTripper
	redirect map[string]string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if dst, ok := m.redirect[req.URL.String()]; ok {
		newReq := req.Clone(req.Context())
		newURL, err := newReq.URL.Parse(dst)
		if err != nil {
			return nil, err
		}
		newReq.URL = newURL
		newReq.Host = newURL.Host
		return m.base.RoundTrip(newReq)
	}
	return m.base.RoundTrip(req)
}

func readReqBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	defer func() { _ = r.Body.Close() }()
	buf := make([]byte, 8192)
	n, _ := r.Body.Read(buf)
	return string(buf[:n])
}
