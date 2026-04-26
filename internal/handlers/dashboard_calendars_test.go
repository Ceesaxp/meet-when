package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/middleware"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
	"github.com/meet-when/meet-when/internal/services"
)

// seedDashboardCalendarFixture creates a tenant + host + connection + a single
// provider_calendars row, returning the host (with tenant) and the calendar id.
// It's a thin convenience for handler-level tests that need a logged-in host
// and an addressable sub-calendar.
func seedDashboardCalendarFixture(t *testing.T, repos *repository.Repositories) (*services.HostWithTenant, string) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.New().String()[:8]

	tenant := &models.Tenant{ID: uuid.New().String(), Slug: "t-" + suffix, Name: "T", CreatedAt: models.Now(), UpdatedAt: models.Now()}
	if err := repos.Tenant.Create(ctx, tenant); err != nil {
		t.Fatalf("tenant: %v", err)
	}
	host := &models.Host{
		ID: uuid.New().String(), TenantID: tenant.ID,
		Email: "h-" + suffix + "@x", PasswordHash: "x", Name: "H",
		Slug: "h-" + suffix, Timezone: "UTC",
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Host.Create(ctx, host); err != nil {
		t.Fatalf("host: %v", err)
	}
	conn := &models.CalendarConnection{
		ID: uuid.New().String(), HostID: host.ID, Provider: models.CalendarProviderGoogle,
		Name: "Google", SyncStatus: models.CalendarSyncStatusUnknown,
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Calendar.Create(ctx, conn); err != nil {
		t.Fatalf("conn: %v", err)
	}
	pc, err := repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, "primary", "Primary", "", true, true)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	return &services.HostWithTenant{Host: host, Tenant: tenant}, pc.ID
}

// requestWithHost builds an authenticated request by stuffing the host into
// the context the way the auth middleware does in production.
func requestWithHost(method, target, body string, host *services.HostWithTenant) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	r = r.WithContext(context.WithValue(r.Context(), middleware.HostKey, host))
	return r
}

// pathValueRequest sets PathValue on an httptest request so handlers that use
// http.Request.PathValue work outside the mux.
func pathValueRequest(method, urlStr, key, value, body string, host *services.HostWithTenant) *http.Request {
	r := requestWithHost(method, urlStr, body, host)
	r.SetPathValue(key, value)
	return r
}

// TestToggleSubCalendarPoll_TogglesAndPersists exercises the new HTMX endpoint:
// posting poll_busy=off must turn off poll_busy on the row, and posting
// poll_busy=on must turn it back on.
func TestToggleSubCalendarPoll_TogglesAndPersists(t *testing.T) {
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	host, pcID := seedDashboardCalendarFixture(t, repos)
	h := createTestHandlers(t, repos)

	// Disable polling.
	form := url.Values{}
	form.Set("poll_busy", "off")
	req := pathValueRequest(http.MethodPost, "/dashboard/calendars/sub/"+pcID+"/poll", "id", pcID, form.Encode(), host)
	w := httptest.NewRecorder()
	h.Dashboard.ToggleSubCalendarPoll(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusSeeOther {
		t.Fatalf("expected 200 or redirect, got %d (body=%s)", w.Code, w.Body.String())
	}

	pc, err := repos.ProviderCalendar.GetByID(context.Background(), pcID)
	if err != nil {
		t.Fatal(err)
	}
	if pc.PollBusy {
		t.Errorf("poll_busy still true after off toggle")
	}

	// Re-enable.
	form.Set("poll_busy", "on")
	req = pathValueRequest(http.MethodPost, "/dashboard/calendars/sub/"+pcID+"/poll", "id", pcID, form.Encode(), host)
	w = httptest.NewRecorder()
	h.Dashboard.ToggleSubCalendarPoll(w, req)

	pc, _ = repos.ProviderCalendar.GetByID(context.Background(), pcID)
	if !pc.PollBusy {
		t.Errorf("poll_busy still false after on toggle")
	}
}

// TestToggleSubCalendarPoll_RejectsForeignHost ensures the handler refuses to
// modify a sub-calendar belonging to a different host.
func TestToggleSubCalendarPoll_RejectsForeignHost(t *testing.T) {
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	_, pcID := seedDashboardCalendarFixture(t, repos)
	otherHost, _ := seedDashboardCalendarFixture(t, repos)
	h := createTestHandlers(t, repos)

	form := url.Values{}
	form.Set("poll_busy", "off")
	req := pathValueRequest(http.MethodPost, "/dashboard/calendars/sub/"+pcID+"/poll", "id", pcID, form.Encode(), otherHost)
	w := httptest.NewRecorder()
	h.Dashboard.ToggleSubCalendarPoll(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 from foreign host, got %d", w.Code)
	}

	// Calendar should still have poll_busy=true.
	pc, _ := repos.ProviderCalendar.GetByID(context.Background(), pcID)
	if !pc.PollBusy {
		t.Errorf("poll_busy was modified by a foreign host")
	}
}

// TestUpdateSubCalendarColor_ValidatesPalette ensures only colors from the
// canonical palette are accepted.
func TestUpdateSubCalendarColor_ValidatesPalette(t *testing.T) {
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	host, pcID := seedDashboardCalendarFixture(t, repos)
	h := createTestHandlers(t, repos)

	form := url.Values{}
	form.Set("color", "#FF00FF") // not in palette
	req := pathValueRequest(http.MethodPost, "/dashboard/calendars/sub/"+pcID+"/color", "id", pcID, form.Encode(), host)
	w := httptest.NewRecorder()
	h.Dashboard.UpdateSubCalendarColor(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for off-palette color, got %d", w.Code)
	}

	// Valid palette color: should succeed (the handler renders a partial; we
	// only assert no error status here since templates aren't loaded).
	form.Set("color", services.CalendarPalette[0])
	req = pathValueRequest(http.MethodPost, "/dashboard/calendars/sub/"+pcID+"/color", "id", pcID, form.Encode(), host)
	w = httptest.NewRecorder()
	h.Dashboard.UpdateSubCalendarColor(w, req)
	// Without templates loaded the handler will fail to render, returning 500;
	// instead we just verify the DB update happened regardless.
	pc, _ := repos.ProviderCalendar.GetByID(context.Background(), pcID)
	if pc.Color != services.CalendarPalette[0] {
		t.Errorf("color not persisted: got %s want %s", pc.Color, services.CalendarPalette[0])
	}
}
