package handlers

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/middleware"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
	"github.com/meet-when/meet-when/internal/services"
)

// loadTestPartials populates the handler's template cache with the partials
// from ../../templates/partials/. We only need partials for HTMX-style tests;
// full page templates require layouts and are already covered by other tests.
func loadTestPartials(t *testing.T, h *Handlers) {
	t.Helper()
	partialFiles, err := filepath.Glob("../../templates/partials/*.html")
	if err != nil || len(partialFiles) == 0 {
		t.Fatalf("partials glob: err=%v matches=%d", err, len(partialFiles))
	}
	if h.templates == nil {
		h.templates = map[string]*template.Template{}
	}
	partialSet, err := template.New("").Funcs(templateFuncs()).ParseFiles(partialFiles...)
	if err != nil {
		t.Fatalf("parse partials: %v", err)
	}
	for _, f := range partialFiles {
		name := filepath.Base(f)
		if partialSet.Lookup(name) != nil {
			h.templates[name] = partialSet
		}
	}
}

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

// TestSetDefaultSubCalendar_PersistsAndRejectsForeignHost covers the new
// per-sub-calendar default-calendar endpoint: the host's default_calendar_id
// must be updated to the sub-calendar's id, and the request must be rejected
// when issued by a foreign host.
func TestSetDefaultSubCalendar_PersistsAndRejectsForeignHost(t *testing.T) {
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	host, pcID := seedDashboardCalendarFixture(t, repos)
	h := createTestHandlers(t, repos)

	req := pathValueRequest(http.MethodPost, "/dashboard/calendars/sub/"+pcID+"/default", "id", pcID, "", host)
	w := httptest.NewRecorder()
	h.Dashboard.SetDefaultSubCalendar(w, req)

	// Without templates loaded the partial render returns 500 even on success;
	// regardless, the DB write must have happened.
	updated, err := repos.Host.GetByID(context.Background(), host.Host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DefaultCalendarID == nil || *updated.DefaultCalendarID != pcID {
		t.Errorf("default_calendar_id not updated: got %v, want %s", updated.DefaultCalendarID, pcID)
	}

	// Foreign host cannot promote someone else's calendar.
	other, _ := seedDashboardCalendarFixture(t, repos)
	req = pathValueRequest(http.MethodPost, "/dashboard/calendars/sub/"+pcID+"/default", "id", pcID, "", other)
	w = httptest.NewRecorder()
	h.Dashboard.SetDefaultSubCalendar(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for foreign host, got %d", w.Code)
	}
}

// addConnection seeds an additional calendar_connections row + one writable
// provider_calendars child for an already-created host. Used by the
// cross-connection default-move test below.
func addConnection(t *testing.T, repos *repository.Repositories, hostID, label string) (connID, pcID string) {
	t.Helper()
	ctx := context.Background()
	conn := &models.CalendarConnection{
		ID: uuid.New().String(), HostID: hostID, Provider: models.CalendarProviderGoogle,
		Name: label, SyncStatus: models.CalendarSyncStatusUnknown,
		CreatedAt: models.Now(), UpdatedAt: models.Now(),
	}
	if err := repos.Calendar.Create(ctx, conn); err != nil {
		t.Fatalf("conn: %v", err)
	}
	pc, err := repos.ProviderCalendar.UpsertFromProvider(ctx, conn.ID, "primary-"+label, label, "", true, true)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	return conn.ID, pc.ID
}

// TestSetDefaultSubCalendar_OOBRefreshOldConnectionCard guards against a UI
// regression: when the host promotes a default that lives under a *different*
// connection than the previous default, BOTH connection cards must end up in
// the response so HTMX clears the stale "default" badge from the old card.
// Regression: previously only the new-default card was returned, leaving two
// cards visibly marked default until a full page reload.
func TestSetDefaultSubCalendar_OOBRefreshOldConnectionCard(t *testing.T) {
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	host, oldPCID := seedDashboardCalendarFixture(t, repos)

	// Seed a second connection with a writable calendar under the same host.
	newConnID, newPCID := addConnection(t, repos, host.Host.ID, "secondary")

	// Make the OLD calendar (in the first connection) the current default.
	if err := repos.Host.SetDefaultCalendar(context.Background(), host.Host.ID, oldPCID); err != nil {
		t.Fatalf("seed default: %v", err)
	}
	// Re-load host so the request-scoped struct sees the seeded default.
	freshHost, err := repos.Host.GetByID(context.Background(), host.Host.ID)
	if err != nil {
		t.Fatalf("reload host: %v", err)
	}
	host.Host = freshHost

	h := createTestHandlers(t, repos)
	loadTestPartials(t, h)

	// Promote the calendar in the SECOND connection.
	req := pathValueRequest(http.MethodPost, "/dashboard/calendars/sub/"+newPCID+"/default", "id", newPCID, "", host)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.Dashboard.SetDefaultSubCalendar(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	body := w.Body.String()

	// The new connection's card must be present as the primary swap target
	// (no hx-swap-oob attribute, since it lands at the hx-target of the form).
	newCardOpen := `id="calendar-card-` + newConnID + `"`
	if !strings.Contains(body, newCardOpen) {
		t.Errorf("new-default connection card missing from response; body=%s", body)
	}

	// The old connection's card must ALSO be present, marked for OOB swap.
	oldConnID := ""
	conns, _ := repos.Calendar.GetByHostID(context.Background(), host.Host.ID)
	for _, c := range conns {
		if c.ID != newConnID {
			oldConnID = c.ID
			break
		}
	}
	if oldConnID == "" {
		t.Fatal("could not locate old connection in seeded data")
	}
	oldCardOpen := `id="calendar-card-` + oldConnID + `" hx-swap-oob="true"`
	if !strings.Contains(body, oldCardOpen) {
		t.Errorf("old connection card missing OOB swap; expected %q in body=%s", oldCardOpen, body)
	}

	// Sanity: the OOB card should not also be rendered without hx-swap-oob —
	// otherwise HTMX would try to use it as the primary target.
	primaryOldCard := `id="calendar-card-` + oldConnID + `">`
	if strings.Contains(body, primaryOldCard) {
		t.Errorf("old connection card was emitted both as primary and OOB; body=%s", body)
	}
}

// TestSetDefaultSubCalendar_NoOOBWhenSameConnection verifies that promoting a
// new default under the same connection as the previous default does NOT emit
// an OOB swap (one card already covers both rows, so a second swap would be
// redundant and could replace the primary target with itself).
func TestSetDefaultSubCalendar_NoOOBWhenSameConnection(t *testing.T) {
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	host, oldPCID := seedDashboardCalendarFixture(t, repos)

	// Add a second writable calendar under the SAME connection.
	conns, _ := repos.Calendar.GetByHostID(context.Background(), host.Host.ID)
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	newPC, err := repos.ProviderCalendar.UpsertFromProvider(context.Background(), conns[0].ID, "another@example.com", "Another", "", false, true)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Set old as default.
	if err := repos.Host.SetDefaultCalendar(context.Background(), host.Host.ID, oldPCID); err != nil {
		t.Fatalf("seed default: %v", err)
	}
	freshHost, _ := repos.Host.GetByID(context.Background(), host.Host.ID)
	host.Host = freshHost

	h := createTestHandlers(t, repos)
	loadTestPartials(t, h)

	req := pathValueRequest(http.MethodPost, "/dashboard/calendars/sub/"+newPC.ID+"/default", "id", newPC.ID, "", host)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	h.Dashboard.SetDefaultSubCalendar(w, req)

	body := w.Body.String()
	if strings.Contains(body, `hx-swap-oob="true"`) {
		t.Errorf("did not expect OOB swap when default moves within a single connection; body=%s", body)
	}
}

// TestSetDefaultSubCalendar_RejectsReadOnly ensures a non-writable calendar
// can't be promoted to default — pooled-host event creation requires write
// access on the default.
func TestSetDefaultSubCalendar_RejectsReadOnly(t *testing.T) {
	_, repos, cleanup := setupTestDatabase(t)
	defer cleanup()

	host, _ := seedDashboardCalendarFixture(t, repos)
	h := createTestHandlers(t, repos)

	// Add a read-only sub-calendar under the same connection.
	conns, _ := repos.Calendar.GetByHostID(context.Background(), host.Host.ID)
	if len(conns) == 0 {
		t.Fatal("no connections seeded")
	}
	readonly, err := repos.ProviderCalendar.UpsertFromProvider(context.Background(), conns[0].ID, "ro@example.com", "Read Only", "", false, false /* not writable */)
	if err != nil {
		t.Fatal(err)
	}

	req := pathValueRequest(http.MethodPost, "/dashboard/calendars/sub/"+readonly.ID+"/default", "id", readonly.ID, "", host)
	w := httptest.NewRecorder()
	h.Dashboard.SetDefaultSubCalendar(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for read-only calendar, got %d", w.Code)
	}

	// Default must NOT have been updated.
	updated, _ := repos.Host.GetByID(context.Background(), host.Host.ID)
	if updated.DefaultCalendarID != nil && *updated.DefaultCalendarID == readonly.ID {
		t.Error("default was set to read-only calendar despite rejection")
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
