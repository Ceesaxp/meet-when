package handlers

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meet-when/meet-when/internal/models"
)

// loadHostedEventPartial loads a single partial template with the standard
// templateFuncs FuncMap. Used by the partial-render tests below.
func loadHostedEventPartial(t *testing.T, name string) *template.Template {
	t.Helper()
	tmpl, err := template.New(name).Funcs(templateFuncs()).ParseFiles(
		filepath.Join("..", "..", "templates", "partials", name),
	)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return tmpl
}

// renderTemplateNamed executes a named template into a string. Lets us call
// {{define "content"}}...{{end}} blocks directly without dragging in layouts.
func renderTemplateNamed(t *testing.T, tmpl *template.Template, name string, data interface{}) string {
	t.Helper()
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
	return buf.String()
}

// ---------------------------------------------------------------------------
// event_attendee_picker_results.html
// ---------------------------------------------------------------------------

func TestEventAttendeePickerResults_RendersContacts(t *testing.T) {
	tmpl := loadHostedEventPartial(t, "event_attendee_picker_results.html")
	data := map[string]interface{}{
		"Contacts": []*models.Contact{
			{ID: "c1", Name: "Alice", Email: "alice@example.com"},
			{ID: "c2", Name: "Bob", Email: "bob@example.com"},
		},
		"Query": "al",
	}
	out := renderTemplateNamed(t, tmpl, "event_attendee_picker_results.html", data)

	// Expect a button per contact with email + name.
	for _, want := range []string{"alice@example.com", "Alice", "bob@example.com", "Bob", "addAttendeeFromContact"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q; got: %s", want, out)
		}
	}
}

func TestEventAttendeePickerResults_EmptyShowsFreeFormHint(t *testing.T) {
	tmpl := loadHostedEventPartial(t, "event_attendee_picker_results.html")
	data := map[string]interface{}{
		"Contacts": nil,
		"Query":    "newperson@example.com",
	}
	out := renderTemplateNamed(t, tmpl, "event_attendee_picker_results.html", data)

	if !strings.Contains(out, "free-form") {
		t.Errorf("expected free-form hint when query has no contact match; got: %s", out)
	}
	if !strings.Contains(out, "newperson@example.com") {
		t.Errorf("expected query echoed in hint; got: %s", out)
	}
}

func TestEventAttendeePickerResults_EmptyAndNoQuery_RendersNothing(t *testing.T) {
	tmpl := loadHostedEventPartial(t, "event_attendee_picker_results.html")
	data := map[string]interface{}{
		"Contacts": nil,
		"Query":    "",
	}
	out := renderTemplateNamed(t, tmpl, "event_attendee_picker_results.html", data)

	out = strings.TrimSpace(out)
	if out != "" {
		t.Errorf("expected empty output for empty query + no contacts; got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// event_conflict_warning.html
// ---------------------------------------------------------------------------

func TestEventConflictWarning_RendersConflicts(t *testing.T) {
	tmpl := loadHostedEventPartial(t, "event_conflict_warning.html")
	start1 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	start2 := time.Date(2026, 6, 1, 14, 30, 0, 0, time.UTC)
	data := map[string]interface{}{
		"Conflicts": []models.TimeSlot{
			{Start: start1, End: start1.Add(30 * time.Minute)},
			{Start: start2, End: start2.Add(45 * time.Minute)},
		},
		"HostTimezone": "UTC",
	}
	out := renderTemplateNamed(t, tmpl, "event_conflict_warning.html", data)

	if !strings.Contains(out, "overlaps with 2 existing events") {
		t.Errorf("expected pluralised count; got: %s", out)
	}
	if !strings.Contains(out, "soft warning") {
		t.Errorf("expected 'soft warning' verbiage; got: %s", out)
	}
}

func TestEventConflictWarning_NoConflicts_RendersEmpty(t *testing.T) {
	tmpl := loadHostedEventPartial(t, "event_conflict_warning.html")
	data := map[string]interface{}{
		"Conflicts":    nil,
		"HostTimezone": "UTC",
	}
	out := renderTemplateNamed(t, tmpl, "event_conflict_warning.html", data)
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty output with no conflicts; got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// events_table_partial.html — list rendering with status routing
// ---------------------------------------------------------------------------

func TestEventsTablePartial_RendersScheduledAndCancelled(t *testing.T) {
	tmpl := loadHostedEventPartial(t, "events_table_partial.html")

	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	scheduled := &models.HostedEvent{
		ID: "evt-1", Title: "Quarterly review",
		StartTime: models.NewSQLiteTime(now), EndTime: models.NewSQLiteTime(now.Add(30 * time.Minute)),
		Duration: 30, Status: models.HostedEventStatusScheduled,
		ConferenceLink: "https://meet.google.com/xyz",
	}
	cancelled := &models.HostedEvent{
		ID: "evt-2", Title: "Old standup",
		StartTime: models.NewSQLiteTime(now.Add(-72 * time.Hour)), EndTime: models.NewSQLiteTime(now.Add(-72*time.Hour + 30*time.Minute)),
		Duration: 30, Status: models.HostedEventStatusCancelled,
	}

	data := map[string]interface{}{
		"Events":       []*models.HostedEvent{scheduled, cancelled},
		"HostTimezone": "UTC",
	}
	out := renderTemplateNamed(t, tmpl, "events_table_partial.html", data)

	// Scheduled event: confirmed-style badge + Join button + Edit link + Cancel form
	if !strings.Contains(out, "badge-confirmed") {
		t.Errorf("scheduled events should render confirmed-style badge; got: %s", out)
	}
	if !strings.Contains(out, "Quarterly review") {
		t.Error("scheduled event title missing")
	}
	if !strings.Contains(out, "https://meet.google.com/xyz") {
		t.Error("conference link missing")
	}
	if !strings.Contains(out, "/dashboard/events/evt-1/edit") {
		t.Error("edit link to scheduled event missing")
	}
	if !strings.Contains(out, "/dashboard/events/evt-1/cancel") {
		t.Error("cancel form for scheduled event missing")
	}

	// Cancelled event: cancelled-style badge, no join/edit/cancel actions
	if !strings.Contains(out, "badge-cancelled") {
		t.Errorf("cancelled events should render cancelled-style badge; got: %s", out)
	}
	if strings.Contains(out, "/dashboard/events/evt-2/edit") {
		t.Error("cancelled event should not show an edit link")
	}
	if strings.Contains(out, "/dashboard/events/evt-2/cancel") {
		t.Error("cancelled event should not show a cancel form")
	}
}

// ---------------------------------------------------------------------------
// dashboard_event_form.html — the create/edit form
// ---------------------------------------------------------------------------

func loadEventFormPage(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := template.New("dashboard_event_form.html").Funcs(templateFuncs()).ParseFiles(
		filepath.Join("..", "..", "templates", "pages", "dashboard_event_form.html"),
	)
	if err != nil {
		t.Fatalf("parse form template: %v", err)
	}
	return tmpl
}

func TestEventForm_NewMode_RendersTitleAndAttendeePicker(t *testing.T) {
	tmpl := loadEventFormPage(t)

	data := PageData{
		Title:  "Schedule Event",
		Host:   &models.Host{ID: "h1", Slug: "host", Name: "Host", Timezone: "UTC"},
		Tenant: &models.Tenant{ID: "t1", Slug: "t", Name: "Tenant"},
		Data: map[string]interface{}{
			"IsNew":           true,
			"Event":           nil,
			"Attendees":       nil,
			"Templates":       nil,
			"CalendarOptions": nil,
			"HostTimezone":    "UTC",
		},
	}
	// We render the "content" block so we don't need the dashboard layout.
	out := renderTemplateNamed(t, tmpl, "content", data)

	for _, want := range []string{
		"Schedule Event", // page title
		"action=\"/dashboard/events\"",
		"name=\"title\"",
		"name=\"start_date\"",
		"name=\"start_time\"",
		"name=\"duration\"",
		"name=\"location_type\"",
		"name=\"calendar_id\"",
		"id=\"attendee-search\"",
		"/dashboard/events/attendee-search",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("new-mode form missing %q", want)
		}
	}
	// The hx-include selector references #exclude_event_id but the actual
	// hidden input only renders in edit mode. Look for the input tag itself.
	if strings.Contains(out, `id="exclude_event_id"`) {
		t.Error("new-mode form should not have an exclude_event_id hidden input")
	}
}

func TestEventForm_EditMode_PrefillsAndCarriesExcludeEventID(t *testing.T) {
	tmpl := loadEventFormPage(t)

	start := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	event := &models.HostedEvent{
		ID: "evt-99", Title: "Already scheduled",
		StartTime: models.NewSQLiteTime(start), EndTime: models.NewSQLiteTime(start.Add(45 * time.Minute)),
		Duration: 45, Timezone: "UTC", LocationType: models.ConferencingProviderZoom,
		CalendarID: "cal-1", Description: "Pre-existing description",
	}
	attendees := []*models.HostedEventAttendee{
		{ID: "a1", Email: "alice@example.com", Name: "Alice"},
	}

	data := PageData{
		Title:  "Edit Event",
		Host:   &models.Host{ID: "h1", Slug: "host", Name: "Host", Timezone: "UTC"},
		Tenant: &models.Tenant{ID: "t1", Slug: "t", Name: "Tenant"},
		Data: map[string]interface{}{
			"IsNew":           false,
			"Event":           event,
			"Attendees":       attendees,
			"Templates":       nil,
			"CalendarOptions": nil,
			"HostTimezone":    "UTC",
		},
	}
	out := renderTemplateNamed(t, tmpl, "content", data)

	for _, want := range []string{
		"Edit Event",                                   // header
		"/dashboard/events/evt-99/edit",                // form action targets edit endpoint
		"value=\"Already scheduled\"",                  // title prefilled
		"value=\"2026-06-01\"",                         // date prefilled
		"value=\"10:00\"",                              // time prefilled
		"name=\"exclude_event_id\" value=\"evt-99\"",   // self-collision exclusion in conflict check
		"alice@example.com",                            // attendee chip
	} {
		if !strings.Contains(out, want) {
			t.Errorf("edit-mode form missing %q\nfull output:\n%s", want, out)
		}
	}
	// Zoom should be selected in the radio cards.
	if !strings.Contains(out, "value=\"zoom\" checked") {
		t.Errorf("expected zoom radio to be checked; got: %s", out)
	}
	// 45-minute duration chip should be selected.
	if !strings.Contains(out, "value=\"45\" checked") {
		t.Errorf("expected 45-min duration to be selected; got: %s", out)
	}
}

func TestEventForm_FormErrorRendersHumanCopy(t *testing.T) {
	tmpl := loadEventFormPage(t)

	data := PageData{
		Title:  "Schedule Event",
		Host:   &models.Host{ID: "h1", Slug: "host", Name: "Host", Timezone: "UTC"},
		Tenant: &models.Tenant{ID: "t1", Slug: "t", Name: "Tenant"},
		Data: map[string]interface{}{
			"IsNew":           true,
			"Event":           nil,
			"Attendees":       nil,
			"Templates":       nil,
			"CalendarOptions": nil,
			"HostTimezone":    "UTC",
			"FormError":       "at_least_one_attendee",
		},
	}
	out := renderTemplateNamed(t, tmpl, "content", data)

	if !strings.Contains(out, "Please add at least one attendee") {
		t.Errorf("expected human-readable error for at_least_one_attendee; got: %s", out)
	}
	if !strings.Contains(out, "alert-error") {
		t.Errorf("expected alert-error styling; got: %s", out)
	}
}
