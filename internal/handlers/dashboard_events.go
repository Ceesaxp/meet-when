package handlers

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/meet-when/meet-when/internal/middleware"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/services"
)

// DashboardEventsHandler handles host-driven event scheduling routes.
type DashboardEventsHandler struct {
	handlers *Handlers
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// List renders the host's hosted events.
func (h *DashboardEventsHandler) List(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	includeArchived := r.URL.Query().Get("archived") == "true"
	events, err := h.handlers.services.HostedEvent.List(r.Context(), host.Host.ID, includeArchived)
	if err != nil {
		log.Printf("[EVENTS] Error listing events: %v", err)
	}

	h.handlers.render(w, "dashboard_events.html", PageData{
		Title:        "Schedule Event",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "events",
		PendingCount: h.handlers.Dashboard.getPendingCount(r, host.Host.ID),
		BaseURL:      h.handlers.cfg.Server.BaseURL,
		Data: map[string]interface{}{
			"Events":        events,
			"ShowArchived":  includeArchived,
			"HostTimezone":  host.Host.Timezone,
		},
	})
}

// ---------------------------------------------------------------------------
// New form
// ---------------------------------------------------------------------------

// NewForm renders the create-event form. ?template=ID prefills duration,
// conferencing provider, and calendar from the named template.
func (h *DashboardEventsHandler) NewForm(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	calendarOptions, _ := h.handlers.services.Calendar.GetCalendarTree(r.Context(), host.Host.ID)
	templates, _ := h.handlers.services.Template.GetTemplates(r.Context(), host.Host.ID)

	var prefill *models.MeetingTemplate
	if templateID := r.URL.Query().Get("template"); templateID != "" {
		t, _ := h.handlers.services.Template.GetTemplate(r.Context(), host.Host.ID, templateID)
		prefill = t
	}

	h.handlers.render(w, "dashboard_event_form.html", PageData{
		Title:        "Schedule Event",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "events",
		PendingCount: h.handlers.Dashboard.getPendingCount(r, host.Host.ID),
		BaseURL:      h.handlers.cfg.Server.BaseURL,
		Data: map[string]interface{}{
			"IsNew":           true,
			"Event":           nil,
			"Attendees":       nil,
			"CalendarOptions": calendarOptions,
			"Templates":       templates,
			"Prefill":         prefill,
			"HostTimezone":    host.Host.Timezone,
		},
	})
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// Create handles event creation.
func (h *DashboardEventsHandler) Create(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/dashboard/events/new?error=invalid_form")
		return
	}

	input, formErr := h.parseCreateInput(r, host.Host.ID, host.Tenant.ID, host.Host.Timezone)
	if formErr != "" {
		h.renderFormError(w, r, host, formErr, nil)
		return
	}

	if _, err := h.handlers.services.HostedEvent.Create(r.Context(), input); err != nil {
		log.Printf("[EVENTS] Create failed: %v", err)
		h.renderFormError(w, r, host, err.Error(), &input)
		return
	}

	h.handlers.redirect(w, r, "/dashboard/events")
}

// ---------------------------------------------------------------------------
// Details / Edit / Update
// ---------------------------------------------------------------------------

// Details renders the event detail modal partial. Loaded via fetch from the
// list page (matches the booking modal pattern).
func (h *DashboardEventsHandler) Details(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	eventID := r.PathValue("id")
	details, err := h.handlers.services.HostedEvent.Get(r.Context(), host.Host.ID, host.Tenant.ID, eventID)
	if err != nil || details == nil {
		http.Error(w, "event not found", http.StatusNotFound)
		return
	}

	h.handlers.renderPartial(w, "dashboard_event_detail_partial.html", map[string]interface{}{
		"Event":        details.Event,
		"Host":         details.Host,
		"Tenant":       details.Tenant,
		"Template":     details.Template,
		"Attendees":    details.Attendees,
		"HostTimezone": host.Host.Timezone,
	})
}

// EditForm renders the edit form (full page, not modal — edits are richer
// than the booking-edit modal supports).
func (h *DashboardEventsHandler) EditForm(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	eventID := r.PathValue("id")
	details, err := h.handlers.services.HostedEvent.Get(r.Context(), host.Host.ID, host.Tenant.ID, eventID)
	if err != nil || details == nil {
		h.handlers.error(w, r, http.StatusNotFound, "Event not found")
		return
	}

	calendarOptions, _ := h.handlers.services.Calendar.GetCalendarTree(r.Context(), host.Host.ID)
	templates, _ := h.handlers.services.Template.GetTemplates(r.Context(), host.Host.ID)

	h.handlers.render(w, "dashboard_event_form.html", PageData{
		Title:        "Edit Event",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "events",
		PendingCount: h.handlers.Dashboard.getPendingCount(r, host.Host.ID),
		BaseURL:      h.handlers.cfg.Server.BaseURL,
		Data: map[string]interface{}{
			"IsNew":           false,
			"Event":           details.Event,
			"Attendees":       details.Attendees,
			"Template":        details.Template,
			"CalendarOptions": calendarOptions,
			"Templates":       templates,
			"HostTimezone":    host.Host.Timezone,
		},
	})
}

// Update handles event updates. Uses POST (not PUT) for HTML form
// compatibility, matching the booking edit pattern.
func (h *DashboardEventsHandler) Update(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	eventID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/dashboard/events/"+eventID+"/edit?error=invalid_form")
		return
	}

	input, formErr := h.parseUpdateInput(r, host.Host.ID, host.Tenant.ID, eventID, host.Host.Timezone)
	if formErr != "" {
		h.handlers.redirect(w, r, "/dashboard/events/"+eventID+"/edit?error="+url.QueryEscape(formErr))
		return
	}

	_, _, err := h.handlers.services.HostedEvent.Update(r.Context(), input)
	if err != nil {
		if errors.Is(err, services.ErrConferencingReauthRequired) {
			h.handlers.redirect(w, r, "/dashboard/events/"+eventID+"/edit?error=reauth_required")
			return
		}
		log.Printf("[EVENTS] Update failed: %v", err)
		h.handlers.redirect(w, r, "/dashboard/events/"+eventID+"/edit?error="+url.QueryEscape(err.Error()))
		return
	}

	h.handlers.redirect(w, r, "/dashboard/events")
}

// ---------------------------------------------------------------------------
// Cancel / Archive / Unarchive / RetryCalendar
// ---------------------------------------------------------------------------

// Cancel cancels a hosted event.
func (h *DashboardEventsHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	eventID := r.PathValue("id")
	reason := r.FormValue("reason")
	if err := h.handlers.services.HostedEvent.Cancel(r.Context(), host.Host.ID, host.Tenant.ID, eventID, reason); err != nil {
		log.Printf("[EVENTS] Cancel failed: %v", err)
	}
	h.handlers.redirect(w, r, "/dashboard/events")
}

// Archive marks an event as archived.
func (h *DashboardEventsHandler) Archive(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}
	eventID := r.PathValue("id")
	if err := h.handlers.services.HostedEvent.Archive(r.Context(), host.Host.ID, host.Tenant.ID, eventID); err != nil {
		log.Printf("[EVENTS] Archive failed: %v", err)
	}
	h.handlers.redirect(w, r, "/dashboard/events")
}

// Unarchive removes the archived flag.
func (h *DashboardEventsHandler) Unarchive(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}
	eventID := r.PathValue("id")
	if err := h.handlers.services.HostedEvent.Unarchive(r.Context(), host.Host.ID, host.Tenant.ID, eventID); err != nil {
		log.Printf("[EVENTS] Unarchive failed: %v", err)
	}
	h.handlers.redirect(w, r, "/dashboard/events?archived=true")
}

// RetryCalendar re-runs calendar event creation for an event.
func (h *DashboardEventsHandler) RetryCalendar(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}
	eventID := r.PathValue("id")
	if err := h.handlers.services.HostedEvent.RetryCalendarEvent(r.Context(), host.Host.ID, host.Tenant.ID, eventID); err != nil {
		log.Printf("[EVENTS] RetryCalendar failed: %v", err)
	}
	h.handlers.redirect(w, r, "/dashboard/events")
}

// ---------------------------------------------------------------------------
// HTMX partials
// ---------------------------------------------------------------------------

// CheckConflicts returns the conflict-warning partial for a proposed time
// window. Used by the form's date/time inputs via HTMX.
//
// Query params: start_date (YYYY-MM-DD), start_time (HH:MM), duration (min),
// timezone (IANA), exclude_event_id (optional).
func (h *DashboardEventsHandler) CheckConflicts(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tz := r.URL.Query().Get("timezone")
	if tz == "" {
		tz = host.Host.Timezone
	}
	startTime, err := parseDateTimeInTZ(r.URL.Query().Get("start_date"), r.URL.Query().Get("start_time"), tz)
	if err != nil {
		// Render an empty partial — no warning when input is incomplete.
		h.handlers.renderPartial(w, "event_conflict_warning.html", map[string]interface{}{
			"Conflicts": nil,
		})
		return
	}
	duration, _ := strconv.Atoi(r.URL.Query().Get("duration"))
	if duration <= 0 {
		duration = 30
	}
	endTime := startTime.Add(time.Duration(duration) * time.Minute)

	excludeID := r.URL.Query().Get("exclude_event_id")
	conflicts, err := h.handlers.services.HostedEvent.DetectBusyConflicts(r.Context(), host.Host.ID, startTime, endTime, excludeID)
	if err != nil {
		log.Printf("[EVENTS] DetectBusyConflicts: %v", err)
	}

	h.handlers.renderPartial(w, "event_conflict_warning.html", map[string]interface{}{
		"Conflicts":    conflicts,
		"HostTimezone": host.Host.Timezone,
	})
}

// AttendeeSearch returns matching contacts for the attendee picker. Empty
// or single-character queries return an empty result.
func (h *DashboardEventsHandler) AttendeeSearch(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var contacts []*models.Contact
	if len(q) >= 2 {
		var err error
		contacts, err = h.handlers.services.Contact.ListContacts(r.Context(), host.Tenant.ID, q, 0, 8)
		if err != nil {
			log.Printf("[EVENTS] AttendeeSearch: %v", err)
		}
	}

	h.handlers.renderPartial(w, "event_attendee_picker_results.html", map[string]interface{}{
		"Contacts": contacts,
		"Query":    q,
	})
}

// ---------------------------------------------------------------------------
// Form parsing helpers
// ---------------------------------------------------------------------------

// parseCreateInput converts the form into a CreateHostedEventInput. Returns
// the input and a non-empty form-error string when validation fails.
func (h *DashboardEventsHandler) parseCreateInput(r *http.Request, hostID, tenantID, defaultTZ string) (services.CreateHostedEventInput, string) {
	tz := r.FormValue("timezone")
	if tz == "" {
		tz = defaultTZ
	}
	startTime, err := parseDateTimeInTZ(r.FormValue("start_date"), r.FormValue("start_time"), tz)
	if err != nil {
		return services.CreateHostedEventInput{}, "invalid_start_time"
	}

	duration, _ := strconv.Atoi(r.FormValue("duration"))
	if duration <= 0 {
		return services.CreateHostedEventInput{}, "invalid_duration"
	}

	attendees, attErr := parseAttendeesFromForm(r)
	if attErr != "" {
		return services.CreateHostedEventInput{}, attErr
	}
	if len(attendees) == 0 {
		return services.CreateHostedEventInput{}, "at_least_one_attendee"
	}

	var templateID *string
	if v := r.FormValue("template_id"); v != "" {
		templateID = &v
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		return services.CreateHostedEventInput{}, "title_required"
	}

	return services.CreateHostedEventInput{
		HostID:         hostID,
		TenantID:       tenantID,
		TemplateID:     templateID,
		Title:          title,
		Description:    r.FormValue("description"),
		Start:          startTime,
		Duration:       duration,
		Timezone:       tz,
		LocationType:   models.ConferencingProvider(r.FormValue("location_type")),
		CustomLocation: r.FormValue("custom_location"),
		CalendarID:     r.FormValue("calendar_id"),
		Attendees:      attendees,
	}, ""
}

// parseUpdateInput is the partial-update analogue. Only sets pointer fields
// when the form supplied a value, so the service can distinguish "no change"
// from "set to zero".
func (h *DashboardEventsHandler) parseUpdateInput(r *http.Request, hostID, tenantID, eventID, defaultTZ string) (services.UpdateHostedEventInput, string) {
	out := services.UpdateHostedEventInput{
		HostID:                   hostID,
		TenantID:                 tenantID,
		EventID:                  eventID,
		RegenerateConferenceLink: r.FormValue("regenerate_conference_link") == "on",
	}

	if v := r.FormValue("title"); v != "" {
		title := strings.TrimSpace(v)
		out.Title = &title
	}
	if r.PostForm.Has("description") {
		v := r.FormValue("description")
		out.Description = &v
	}
	if r.PostForm.Has("custom_location") {
		v := r.FormValue("custom_location")
		out.CustomLocation = &v
	}
	if v := r.FormValue("location_type"); v != "" {
		lt := models.ConferencingProvider(v)
		out.LocationType = &lt
	}
	if v := r.FormValue("calendar_id"); v != "" {
		out.CalendarID = &v
	}
	tz := r.FormValue("timezone")
	if tz != "" {
		out.Timezone = &tz
	} else {
		tz = defaultTZ
	}
	if r.FormValue("start_date") != "" && r.FormValue("start_time") != "" {
		startTime, err := parseDateTimeInTZ(r.FormValue("start_date"), r.FormValue("start_time"), tz)
		if err != nil {
			return out, "invalid_start_time"
		}
		out.Start = &startTime
	}
	if v := r.FormValue("duration"); v != "" {
		d, err := strconv.Atoi(v)
		if err != nil || d <= 0 {
			return out, "invalid_duration"
		}
		out.Duration = &d
	}

	if r.PostForm.Has("attendee_email") {
		attendees, attErr := parseAttendeesFromForm(r)
		if attErr != "" {
			return out, attErr
		}
		if len(attendees) == 0 {
			return out, "at_least_one_attendee"
		}
		out.Attendees = &attendees
	}

	return out, ""
}

// parseAttendeesFromForm reads parallel-indexed attendee_email/_name/_contact_id
// repeated form fields and produces an AttendeeInput slice.
func parseAttendeesFromForm(r *http.Request) ([]services.AttendeeInput, string) {
	emails := r.Form["attendee_email"]
	names := r.Form["attendee_name"]
	contactIDs := r.Form["attendee_contact_id"]

	out := make([]services.AttendeeInput, 0, len(emails))
	seen := make(map[string]bool, len(emails))
	for i, raw := range emails {
		email := strings.ToLower(strings.TrimSpace(raw))
		if email == "" {
			continue
		}
		if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
			return nil, "invalid_attendee_email"
		}
		if seen[email] {
			continue
		}
		seen[email] = true

		name := ""
		if i < len(names) {
			name = strings.TrimSpace(names[i])
		}
		var contactID *string
		if i < len(contactIDs) && contactIDs[i] != "" {
			id := contactIDs[i]
			contactID = &id
		}
		out = append(out, services.AttendeeInput{
			Email:     email,
			Name:      name,
			ContactID: contactID,
		})
	}
	return out, ""
}

// parseDateTimeInTZ combines a date string ("YYYY-MM-DD") and a time string
// ("HH:MM") in a named IANA timezone, returning the resulting UTC time.Time.
func parseDateTimeInTZ(dateStr, timeStr, tz string) (time.Time, error) {
	if dateStr == "" || timeStr == "" {
		return time.Time{}, errors.New("date and time required")
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04", dateStr+" "+timeStr, loc)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

// renderFormError re-renders the create form with a flash error and the
// previously-supplied input echoed back (so the host doesn't lose the data
// they already typed).
func (h *DashboardEventsHandler) renderFormError(w http.ResponseWriter, r *http.Request, host *services.HostWithTenant, errMsg string, prior *services.CreateHostedEventInput) {
	calendarOptions, _ := h.handlers.services.Calendar.GetCalendarTree(r.Context(), host.Host.ID)
	templates, _ := h.handlers.services.Template.GetTemplates(r.Context(), host.Host.ID)

	data := map[string]interface{}{
		"IsNew":           true,
		"Event":           nil,
		"Attendees":       nil,
		"CalendarOptions": calendarOptions,
		"Templates":       templates,
		"HostTimezone":    host.Host.Timezone,
		"FormError":       errMsg,
	}
	if prior != nil {
		data["PriorInput"] = prior
	}

	h.handlers.render(w, "dashboard_event_form.html", PageData{
		Title:        "Schedule Event",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "events",
		PendingCount: h.handlers.Dashboard.getPendingCount(r, host.Host.ID),
		BaseURL:      h.handlers.cfg.Server.BaseURL,
		Flash:        &FlashMessage{Type: "error", Message: errMsg},
		Data:         data,
	})
}
