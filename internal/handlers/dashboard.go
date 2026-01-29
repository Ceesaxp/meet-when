package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/middleware"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/services"
)

// DashboardHandler handles dashboard routes
type DashboardHandler struct {
	handlers *Handlers
}

// getPendingCount returns the count of pending bookings for the host
func (h *DashboardHandler) getPendingCount(r *http.Request, hostID string) int {
	pending, _ := h.handlers.services.Booking.GetPendingBookings(r.Context(), hostID)
	return len(pending)
}

// Home renders the dashboard home page
func (h *DashboardHandler) Home(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	// Get upcoming bookings (exclude archived)
	bookings, _ := h.handlers.services.Booking.GetBookings(r.Context(), host.Host.ID, nil, false)

	// Get pending bookings count
	pending, _ := h.handlers.services.Booking.GetPendingBookings(r.Context(), host.Host.ID)

	// Get templates count
	templates, _ := h.handlers.services.Template.GetTemplates(r.Context(), host.Host.ID)

	h.handlers.render(w, "dashboard_home.html", PageData{
		Title:        "Dashboard",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "home",
		PendingCount: len(pending),
		BaseURL:      h.handlers.cfg.Server.BaseURL,
		Data: map[string]interface{}{
			"Bookings":     bookings,
			"PendingCount": len(pending),
			"Templates":    templates,
		},
	})
}

// Calendars renders the calendar connections page
func (h *DashboardHandler) Calendars(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	calendars, err := h.handlers.services.Calendar.GetCalendars(r.Context(), host.Host.ID)
	if err != nil {
		log.Printf("Error fetching calendars: %v", err)
	}
	conferencing, err := h.handlers.services.Conferencing.GetConnections(r.Context(), host.Host.ID)
	if err != nil {
		log.Printf("Error fetching conferences: %v", err)
	}

	// Build OAuth URLs
	googleAuthURL := h.handlers.services.Calendar.GetGoogleAuthURL(host.Host.ID)
	zoomAuthURL := h.handlers.services.Conferencing.GetZoomAuthURL(host.Host.ID)

	h.handlers.render(w, "dashboard_calendars.html", PageData{
		Title:        "Calendars & Integrations",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "calendars",
		PendingCount: h.getPendingCount(r, host.Host.ID),
		Data: map[string]interface{}{
			"Calendars":     calendars,
			"Conferencing":  conferencing,
			"GoogleAuthURL": googleAuthURL,
			"ZoomAuthURL":   zoomAuthURL,
		},
	})
}

// ConnectGoogle initiates Google Calendar OAuth flow
func (h *DashboardHandler) ConnectGoogle(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	authURL := h.handlers.services.Calendar.GetGoogleAuthURL(host.Host.ID)
	log.Printf("Redirecting to Google auth URL: %s", authURL)
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

// ConnectCalDAV handles CalDAV connection form
func (h *DashboardHandler) ConnectCalDAV(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/dashboard/calendars?error=invalid_form")
		return
	}

	// Determine provider from form field (defaults to caldav)
	provider := models.CalendarProvider(r.FormValue("provider"))
	if provider == "" {
		provider = models.CalendarProviderCalDAV
	}

	input := services.CalDAVConnectInput{
		HostID:   host.Host.ID,
		Name:     r.FormValue("name"),
		URL:      r.FormValue("url"),
		Username: r.FormValue("username"),
		Password: r.FormValue("password"),
		Provider: provider,
	}

	_, err := h.handlers.services.Calendar.ConnectCalDAV(r.Context(), input)
	if err != nil {
		h.handlers.redirect(w, r, "/dashboard/calendars?error=connection_failed")
		return
	}

	h.handlers.redirect(w, r, "/dashboard/calendars?success=calendar_connected")
}

// DisconnectCalendar removes a calendar connection
func (h *DashboardHandler) DisconnectCalendar(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	calendarID := r.PathValue("id")
	_ = h.handlers.services.Calendar.DisconnectCalendar(r.Context(), host.Host.ID, calendarID)

	h.handlers.redirect(w, r, "/dashboard/calendars")
}

// RefreshCalendarSync manually triggers a sync check for a calendar
func (h *DashboardHandler) RefreshCalendarSync(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	calendarID := r.PathValue("id")
	err := h.handlers.services.Calendar.RefreshCalendarSync(r.Context(), host.Host.ID, calendarID)

	// For HTMX requests, return the updated calendar card partial
	if r.Header.Get("HX-Request") == "true" {
		// Get the updated calendar data
		cal, calErr := h.handlers.services.Calendar.GetCalendar(r.Context(), calendarID)
		if calErr != nil || cal == nil {
			http.Error(w, "Calendar not found", http.StatusNotFound)
			return
		}

		// If sync failed, the calendar will have the error in SyncError field
		h.handlers.renderPartial(w, "calendar_card_partial.html", cal)
		return
	}

	// For regular requests, redirect as before
	if err != nil {
		log.Printf("Calendar sync refresh failed for %s: %v", calendarID, err)
		h.handlers.redirect(w, r, "/dashboard/calendars?error=sync_failed")
		return
	}

	h.handlers.redirect(w, r, "/dashboard/calendars?success=sync_complete")
}

// SetDefaultCalendar sets a calendar as default
func (h *DashboardHandler) SetDefaultCalendar(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	calendarID := r.PathValue("id")
	_ = h.handlers.services.Calendar.SetDefaultCalendar(r.Context(), host.Host.ID, calendarID)

	h.handlers.redirect(w, r, "/dashboard/calendars")
}

// Templates renders the meeting templates list
func (h *DashboardHandler) Templates(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	templates, _ := h.handlers.services.Template.GetTemplates(r.Context(), host.Host.ID)
	bookingCounts, _ := h.handlers.services.Booking.GetBookingCountsByHostID(r.Context(), host.Host.ID)

	h.handlers.render(w, "dashboard_templates.html", PageData{
		Title:        "Meeting Templates",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "templates",
		PendingCount: h.getPendingCount(r, host.Host.ID),
		BaseURL:      h.handlers.cfg.Server.BaseURL,
		Data: map[string]interface{}{
			"Templates":     templates,
			"BookingCounts": bookingCounts,
		},
	})
}

// NewTemplatePage renders the new template form
func (h *DashboardHandler) NewTemplatePage(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	calendars, _ := h.handlers.services.Calendar.GetCalendars(r.Context(), host.Host.ID)

	h.handlers.render(w, "dashboard_template_form.html", PageData{
		Title:        "New Meeting Template",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "templates",
		PendingCount: h.getPendingCount(r, host.Host.ID),
		Data: map[string]interface{}{
			"Template":  nil,
			"Calendars": calendars,
			"IsNew":     true,
		},
	})
}

// CreateTemplate handles template creation
func (h *DashboardHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/dashboard/templates/new?error=invalid_form")
		return
	}

	// Parse durations
	var durations []int
	for _, d := range r.Form["durations"] {
		if dur, err := strconv.Atoi(d); err == nil {
			durations = append(durations, dur)
		}
	}
	if len(durations) == 0 {
		durations = []int{30}
	}

	// Parse invitee questions
	var inviteeQuestions models.JSONArray
	if questionsJSON := r.FormValue("invitee_questions"); questionsJSON != "" {
		if err := json.Unmarshal([]byte(questionsJSON), &inviteeQuestions); err != nil {
			log.Printf("[TEMPLATE] Failed to parse invitee_questions: %v", err)
		}
	}

	// Parse availability rules
	var availabilityRules models.JSONMap
	if rulesJSON := r.FormValue("availability_rules"); rulesJSON != "" {
		if err := json.Unmarshal([]byte(rulesJSON), &availabilityRules); err != nil {
			log.Printf("[TEMPLATE] Failed to parse availability_rules: %v", err)
		}
	}

	input := services.CreateTemplateInput{
		HostID:            host.Host.ID,
		TenantID:          host.Tenant.ID,
		Slug:              r.FormValue("slug"),
		Name:              r.FormValue("name"),
		Description:       r.FormValue("description"),
		Durations:         durations,
		LocationType:      models.ConferencingProvider(r.FormValue("location_type")),
		CustomLocation:    r.FormValue("custom_location"),
		CalendarID:        r.FormValue("calendar_id"),
		RequiresApproval:  r.FormValue("requires_approval") == "on",
		MinNoticeMinutes:  parseIntOrDefault(r.FormValue("min_notice_minutes"), 60),
		MaxScheduleDays:   parseIntOrDefault(r.FormValue("max_schedule_days"), 14),
		PreBufferMinutes:  parseIntOrDefault(r.FormValue("pre_buffer_minutes"), 0),
		PostBufferMinutes: parseIntOrDefault(r.FormValue("post_buffer_minutes"), 0),
		AvailabilityRules: availabilityRules,
		InviteeQuestions:  inviteeQuestions,
		ConfirmationEmail: r.FormValue("confirmation_email"),
		ReminderEmail:     r.FormValue("reminder_email"),
		IsPrivate:         r.FormValue("is_private") == "on",
	}

	_, err := h.handlers.services.Template.CreateTemplate(r.Context(), input)
	if err != nil {
		calendars, _ := h.handlers.services.Calendar.GetCalendars(r.Context(), host.Host.ID)
		h.handlers.render(w, "dashboard_template_form.html", PageData{
			Title:        "New Meeting Template",
			Host:         host.Host,
			Tenant:       host.Tenant,
			ActiveNav:    "templates",
			PendingCount: h.getPendingCount(r, host.Host.ID),
			Flash:        &FlashMessage{Type: "error", Message: "Failed to create template: " + err.Error()},
			Data: map[string]interface{}{
				"Template":  nil,
				"Calendars": calendars,
				"IsNew":     true,
			},
		})
		return
	}

	h.handlers.redirect(w, r, "/dashboard/templates")
}

// EditTemplatePage renders the edit template form
func (h *DashboardHandler) EditTemplatePage(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	templateID := r.PathValue("id")
	template, err := h.handlers.services.Template.GetTemplate(r.Context(), host.Host.ID, templateID)
	if err != nil || template == nil {
		h.handlers.error(w, r, http.StatusNotFound, "Template not found")
		return
	}

	calendars, _ := h.handlers.services.Calendar.GetCalendars(r.Context(), host.Host.ID)

	// Load pooled hosts for this template
	pooledHosts, _ := h.handlers.services.Template.GetPooledHosts(r.Context(), templateID)

	// Load all hosts in the tenant for the dropdown, excluding already-pooled hosts
	allTenantHosts, _ := h.handlers.repos.Host.GetByTenantID(r.Context(), host.Tenant.ID)
	tenantHosts := filterAvailableHosts(allTenantHosts, pooledHosts)

	h.handlers.render(w, "dashboard_template_form.html", PageData{
		Title:        "Edit Template",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "templates",
		PendingCount: h.getPendingCount(r, host.Host.ID),
		Data: map[string]interface{}{
			"Template":    template,
			"Calendars":   calendars,
			"IsNew":       false,
			"PooledHosts": pooledHosts,
			"TenantHosts": tenantHosts,
		},
	})
}

// UpdateTemplate handles template updates
func (h *DashboardHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	templateID := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/dashboard/templates/"+templateID+"?error=invalid_form")
		return
	}

	// Parse durations
	var durations []int
	for _, d := range r.Form["durations"] {
		if dur, err := strconv.Atoi(d); err == nil {
			durations = append(durations, dur)
		}
	}
	if len(durations) == 0 {
		durations = []int{30}
	}

	// Parse invitee questions
	var inviteeQuestions models.JSONArray
	if questionsJSON := r.FormValue("invitee_questions"); questionsJSON != "" {
		if err := json.Unmarshal([]byte(questionsJSON), &inviteeQuestions); err != nil {
			log.Printf("[TEMPLATE] Failed to parse invitee_questions: %v", err)
		}
	}

	// Parse availability rules
	var availabilityRules models.JSONMap
	if rulesJSON := r.FormValue("availability_rules"); rulesJSON != "" {
		if err := json.Unmarshal([]byte(rulesJSON), &availabilityRules); err != nil {
			log.Printf("[TEMPLATE] Failed to parse availability_rules: %v", err)
		}
	}

	input := services.UpdateTemplateInput{
		ID:                templateID,
		HostID:            host.Host.ID,
		TenantID:          host.Tenant.ID,
		Slug:              r.FormValue("slug"),
		Name:              r.FormValue("name"),
		Description:       r.FormValue("description"),
		Durations:         durations,
		LocationType:      models.ConferencingProvider(r.FormValue("location_type")),
		CustomLocation:    r.FormValue("custom_location"),
		CalendarID:        r.FormValue("calendar_id"),
		RequiresApproval:  r.FormValue("requires_approval") == "on",
		MinNoticeMinutes:  parseIntOrDefault(r.FormValue("min_notice_minutes"), 60),
		MaxScheduleDays:   parseIntOrDefault(r.FormValue("max_schedule_days"), 14),
		PreBufferMinutes:  parseIntOrDefault(r.FormValue("pre_buffer_minutes"), 0),
		PostBufferMinutes: parseIntOrDefault(r.FormValue("post_buffer_minutes"), 0),
		AvailabilityRules: availabilityRules,
		InviteeQuestions:  inviteeQuestions,
		ConfirmationEmail: r.FormValue("confirmation_email"),
		ReminderEmail:     r.FormValue("reminder_email"),
		IsActive:          r.FormValue("is_active") == "on",
		IsPrivate:         r.FormValue("is_private") == "on",
	}

	_, err := h.handlers.services.Template.UpdateTemplate(r.Context(), input)
	if err != nil {
		h.handlers.redirect(w, r, "/dashboard/templates/"+templateID+"?error=update_failed")
		return
	}

	h.handlers.redirect(w, r, "/dashboard/templates")
}

// DeleteTemplate handles template deletion
func (h *DashboardHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	templateID := r.PathValue("id")
	_ = h.handlers.services.Template.DeleteTemplate(r.Context(), host.Host.ID, host.Tenant.ID, templateID)

	h.handlers.redirect(w, r, "/dashboard/templates")
}

// DuplicateTemplate handles template duplication
func (h *DashboardHandler) DuplicateTemplate(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	templateID := r.PathValue("id")
	duplicate, err := h.handlers.services.Template.DuplicateTemplate(r.Context(), host.Host.ID, host.Tenant.ID, templateID)
	if err != nil {
		h.handlers.redirect(w, r, "/dashboard/templates?error=duplicate_failed")
		return
	}

	// Redirect to edit page for the new template
	h.handlers.redirect(w, r, "/dashboard/templates/"+duplicate.ID)
}

// AddPooledHost adds a host to a template's pool
func (h *DashboardHandler) AddPooledHost(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	templateID := r.PathValue("id")

	// Verify the current host owns this template
	template, err := h.handlers.services.Template.GetTemplate(r.Context(), host.Host.ID, templateID)
	if err != nil || template == nil {
		if r.Header.Get("HX-Request") == "true" {
			http.Error(w, "Template not found", http.StatusNotFound)
			return
		}
		h.handlers.redirect(w, r, "/dashboard/templates")
		return
	}

	if err := r.ParseForm(); err != nil {
		if r.Header.Get("HX-Request") == "true" {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		h.handlers.redirect(w, r, "/dashboard/templates/"+templateID+"?error=invalid_form")
		return
	}

	hostIDToAdd := r.FormValue("host_id")
	isOptional := r.FormValue("is_optional") == "on"

	_, err = h.handlers.services.Template.AddPooledHost(r.Context(), host.Tenant.ID, templateID, hostIDToAdd, isOptional)
	if err != nil {
		if r.Header.Get("HX-Request") == "true" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.handlers.redirect(w, r, "/dashboard/templates/"+templateID+"?error=add_host_failed")
		return
	}

	// For HTMX requests, return the updated pooled hosts partial
	if r.Header.Get("HX-Request") == "true" {
		pooledHosts, _ := h.handlers.services.Template.GetPooledHosts(r.Context(), templateID)
		allTenantHosts, _ := h.handlers.repos.Host.GetByTenantID(r.Context(), host.Tenant.ID)
		tenantHosts := filterAvailableHosts(allTenantHosts, pooledHosts)
		h.handlers.renderPartial(w, "pooled_hosts_partial.html", map[string]interface{}{
			"Template":    template,
			"PooledHosts": pooledHosts,
			"TenantHosts": tenantHosts,
		})
		return
	}

	h.handlers.redirect(w, r, "/dashboard/templates/"+templateID)
}

// RemovePooledHost removes a host from a template's pool
func (h *DashboardHandler) RemovePooledHost(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	templateID := r.PathValue("id")
	hostIDToRemove := r.PathValue("hostId")

	// Verify the current host owns this template
	template, err := h.handlers.services.Template.GetTemplate(r.Context(), host.Host.ID, templateID)
	if err != nil || template == nil {
		if r.Header.Get("HX-Request") == "true" {
			http.Error(w, "Template not found", http.StatusNotFound)
			return
		}
		h.handlers.redirect(w, r, "/dashboard/templates")
		return
	}

	err = h.handlers.services.Template.RemovePooledHost(r.Context(), host.Tenant.ID, templateID, hostIDToRemove)
	if err != nil {
		if r.Header.Get("HX-Request") == "true" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.handlers.redirect(w, r, "/dashboard/templates/"+templateID+"?error=remove_host_failed")
		return
	}

	// For HTMX requests, return the updated pooled hosts partial
	if r.Header.Get("HX-Request") == "true" {
		pooledHosts, _ := h.handlers.services.Template.GetPooledHosts(r.Context(), templateID)
		allTenantHosts, _ := h.handlers.repos.Host.GetByTenantID(r.Context(), host.Tenant.ID)
		tenantHosts := filterAvailableHosts(allTenantHosts, pooledHosts)
		h.handlers.renderPartial(w, "pooled_hosts_partial.html", map[string]interface{}{
			"Template":    template,
			"PooledHosts": pooledHosts,
			"TenantHosts": tenantHosts,
		})
		return
	}

	h.handlers.redirect(w, r, "/dashboard/templates/"+templateID)
}

// UpdatePooledHost updates a pooled host's optional status
func (h *DashboardHandler) UpdatePooledHost(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	templateID := r.PathValue("id")
	hostIDToUpdate := r.PathValue("hostId")

	// Verify the current host owns this template
	template, err := h.handlers.services.Template.GetTemplate(r.Context(), host.Host.ID, templateID)
	if err != nil || template == nil {
		if r.Header.Get("HX-Request") == "true" {
			http.Error(w, "Template not found", http.StatusNotFound)
			return
		}
		h.handlers.redirect(w, r, "/dashboard/templates")
		return
	}

	if err := r.ParseForm(); err != nil {
		if r.Header.Get("HX-Request") == "true" {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		h.handlers.redirect(w, r, "/dashboard/templates/"+templateID+"?error=invalid_form")
		return
	}

	isOptional := r.FormValue("is_optional") == "on"

	err = h.handlers.services.Template.UpdatePooledHost(r.Context(), host.Tenant.ID, templateID, hostIDToUpdate, isOptional)
	if err != nil {
		if r.Header.Get("HX-Request") == "true" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.handlers.redirect(w, r, "/dashboard/templates/"+templateID+"?error=update_host_failed")
		return
	}

	// For HTMX requests, return the updated pooled hosts partial
	if r.Header.Get("HX-Request") == "true" {
		pooledHosts, _ := h.handlers.services.Template.GetPooledHosts(r.Context(), templateID)
		allTenantHosts, _ := h.handlers.repos.Host.GetByTenantID(r.Context(), host.Tenant.ID)
		tenantHosts := filterAvailableHosts(allTenantHosts, pooledHosts)
		h.handlers.renderPartial(w, "pooled_hosts_partial.html", map[string]interface{}{
			"Template":    template,
			"PooledHosts": pooledHosts,
			"TenantHosts": tenantHosts,
		})
		return
	}

	h.handlers.redirect(w, r, "/dashboard/templates/"+templateID)
}

// Bookings renders the bookings list
func (h *DashboardHandler) Bookings(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	filter := r.URL.Query().Get("filter")
	var status *models.BookingStatus
	if filter != "" {
		s := models.BookingStatus(filter)
		status = &s
	}

	// Check if we should include archived bookings
	showArchived := r.URL.Query().Get("archived") == "true"
	bookings, _ := h.handlers.services.Booking.GetBookings(r.Context(), host.Host.ID, status, showArchived)
	templates, _ := h.handlers.services.Template.GetTemplates(r.Context(), host.Host.ID)

	// Create template map for display
	templateMap := make(map[string]*models.MeetingTemplate)
	for _, t := range templates {
		templateMap[t.ID] = t
	}

	// Calculate counts for filter bar (always exclude archived for accurate counts)
	allBookings, _ := h.handlers.services.Booking.GetBookings(r.Context(), host.Host.ID, nil, false)
	pendingStatus := models.BookingStatusPending
	pendingBookings, _ := h.handlers.services.Booking.GetBookings(r.Context(), host.Host.ID, &pendingStatus, false)
	confirmedStatus := models.BookingStatusConfirmed
	confirmedBookings, _ := h.handlers.services.Booking.GetBookings(r.Context(), host.Host.ID, &confirmedStatus, false)
	cancelledStatus := models.BookingStatusCancelled
	cancelledBookings, _ := h.handlers.services.Booking.GetBookings(r.Context(), host.Host.ID, &cancelledStatus, false)

	// Count archivable bookings (cancelled or rejected, not yet archived)
	archivableCount := 0
	for _, b := range allBookings {
		if (b.Status == models.BookingStatusCancelled || b.Status == models.BookingStatusRejected) && !b.IsArchived {
			archivableCount++
		}
	}

	h.handlers.render(w, "dashboard_bookings.html", PageData{
		Title:        "Bookings",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "bookings",
		PendingCount: len(pendingBookings),
		Data: map[string]interface{}{
			"Bookings":        bookings,
			"Templates":       templateMap,
			"Filter":          filter,
			"ShowArchived":    showArchived,
			"AllCount":        len(allBookings),
			"PendingCount":    len(pendingBookings),
			"ConfirmedCount":  len(confirmedBookings),
			"CancelledCount":  len(cancelledBookings),
			"ArchivableCount": archivableCount,
		},
	})
}

// ApproveBooking approves a pending booking
func (h *DashboardHandler) ApproveBooking(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	bookingID := r.PathValue("id")
	_, err := h.handlers.services.Booking.ApproveBooking(r.Context(), host.Host.ID, host.Tenant.ID, bookingID)
	if err != nil {
		h.handlers.redirect(w, r, "/dashboard/bookings?error=approve_failed")
		return
	}

	h.handlers.redirect(w, r, "/dashboard/bookings")
}

// RejectBooking rejects a pending booking
func (h *DashboardHandler) RejectBooking(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/dashboard/bookings?error=invalid_form")
		return
	}

	bookingID := r.PathValue("id")
	reason := r.FormValue("reason")

	err := h.handlers.services.Booking.RejectBooking(r.Context(), host.Host.ID, host.Tenant.ID, bookingID, reason)
	if err != nil {
		h.handlers.redirect(w, r, "/dashboard/bookings?error=reject_failed")
		return
	}

	h.handlers.redirect(w, r, "/dashboard/bookings")
}

// CancelBooking cancels a booking (host action)
func (h *DashboardHandler) CancelBooking(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/dashboard/bookings?error=invalid_form")
		return
	}

	bookingID := r.PathValue("id")
	reason := r.FormValue("reason")

	err := h.handlers.services.Booking.CancelBooking(r.Context(), bookingID, "host", reason)
	if err != nil {
		h.handlers.redirect(w, r, "/dashboard/bookings?error=cancel_failed")
		return
	}

	h.handlers.redirect(w, r, "/dashboard/bookings")
}

// ArchiveBooking archives a cancelled or rejected booking
func (h *DashboardHandler) ArchiveBooking(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	bookingID := r.PathValue("id")
	err := h.handlers.services.Booking.ArchiveBooking(r.Context(), host.Host.ID, host.Tenant.ID, bookingID)

	// For HTMX requests, return empty content (row disappears)
	if r.Header.Get("HX-Request") == "true" {
		if err != nil {
			http.Error(w, "Failed to archive booking", http.StatusBadRequest)
			return
		}
		// Return empty response - the row will be removed via hx-swap="outerHTML"
		w.WriteHeader(http.StatusOK)
		return
	}

	// For regular requests, redirect
	if err != nil {
		h.handlers.redirect(w, r, "/dashboard/bookings?error=archive_failed")
		return
	}

	h.handlers.redirect(w, r, "/dashboard/bookings")
}

// UnarchiveBooking restores an archived booking
func (h *DashboardHandler) UnarchiveBooking(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	bookingID := r.PathValue("id")
	err := h.handlers.services.Booking.UnarchiveBooking(r.Context(), host.Host.ID, host.Tenant.ID, bookingID)

	// For HTMX requests, redirect to refresh the page (we need to reload to show updated row)
	if r.Header.Get("HX-Request") == "true" {
		if err != nil {
			http.Error(w, "Failed to unarchive booking", http.StatusBadRequest)
			return
		}
		// Return HX-Redirect header to reload the page
		w.Header().Set("HX-Redirect", "/dashboard/bookings?archived=true")
		w.WriteHeader(http.StatusOK)
		return
	}

	// For regular requests, redirect
	if err != nil {
		h.handlers.redirect(w, r, "/dashboard/bookings?error=unarchive_failed&archived=true")
		return
	}

	h.handlers.redirect(w, r, "/dashboard/bookings?archived=true")
}

// BulkArchiveBookings archives all cancelled and rejected bookings
func (h *DashboardHandler) BulkArchiveBookings(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	count, err := h.handlers.services.Booking.BulkArchiveBookings(r.Context(), host.Host.ID, host.Tenant.ID)

	// For HTMX requests, redirect to refresh the page
	if r.Header.Get("HX-Request") == "true" {
		if err != nil {
			http.Error(w, "Failed to archive bookings", http.StatusBadRequest)
			return
		}
		// Return HX-Redirect header to reload the page
		w.Header().Set("HX-Redirect", "/dashboard/bookings")
		w.WriteHeader(http.StatusOK)
		return
	}

	// For regular requests, redirect
	if err != nil {
		h.handlers.redirect(w, r, "/dashboard/bookings?error=bulk_archive_failed")
		return
	}

	log.Printf("[DASHBOARD] Bulk archived %d bookings for host %s", count, host.Host.ID)
	h.handlers.redirect(w, r, "/dashboard/bookings")
}

// Settings renders the settings page
func (h *DashboardHandler) Settings(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	workingHours, _ := h.handlers.services.Availability.GetWorkingHours(r.Context(), host.Host.ID)

	// Group working hours by day
	hoursByDay := make(map[int][]*models.WorkingHours)
	for _, wh := range workingHours {
		hoursByDay[wh.DayOfWeek] = append(hoursByDay[wh.DayOfWeek], wh)
	}

	// Check for flash messages from query params
	var flash *FlashMessage
	if r.URL.Query().Get("success") == "updated" {
		flash = &FlashMessage{Type: "success", Message: "Settings saved successfully"}
	} else if errType := r.URL.Query().Get("error"); errType != "" {
		switch errType {
		case "slug_taken":
			flash = &FlashMessage{Type: "error", Message: "That URL slug is already taken"}
		case "update_failed":
			flash = &FlashMessage{Type: "error", Message: "Failed to save settings"}
		case "invalid_form":
			flash = &FlashMessage{Type: "error", Message: "Invalid form data"}
		default:
			flash = &FlashMessage{Type: "error", Message: "An error occurred"}
		}
	}

	h.handlers.render(w, "dashboard_settings.html", PageData{
		Title:        "Settings",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "settings",
		PendingCount: h.getPendingCount(r, host.Host.ID),
		Flash:        flash,
		Data: map[string]interface{}{
			"WorkingHours": hoursByDay,
			"DayNames":     []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
		},
	})
}

// UpdateSettings updates host settings
func (h *DashboardHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/dashboard/settings?error=invalid_form")
		return
	}

	host.Host.Name = r.FormValue("name")
	host.Host.Timezone = r.FormValue("timezone")

	newSlug := r.FormValue("slug")
	if newSlug != "" && newSlug != host.Host.Slug {
		// Validate slug is unique
		existing, _ := h.handlers.services.Auth.GetHostBySlug(r.Context(), host.Tenant.ID, newSlug)
		if existing != nil && existing.ID != host.Host.ID {
			h.handlers.redirect(w, r, "/dashboard/settings?error=slug_taken")
			return
		}
		host.Host.Slug = newSlug
	}

	if err := h.handlers.services.Auth.UpdateHost(r.Context(), host.Host); err != nil {
		h.handlers.redirect(w, r, "/dashboard/settings?error=update_failed")
		return
	}

	h.handlers.redirect(w, r, "/dashboard/settings?success=updated")
}

// UpdateWorkingHours updates working hours and timezone
func (h *DashboardHandler) UpdateWorkingHours(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/dashboard/settings?error=invalid_form")
		return
	}

	// Update timezone if provided
	if timezone := r.FormValue("timezone"); timezone != "" {
		host.Host.Timezone = timezone
		if err := h.handlers.services.Auth.UpdateHost(r.Context(), host.Host); err != nil {
			h.handlers.redirect(w, r, "/dashboard/settings?error=update_failed")
			return
		}
	}

	now := models.Now()
	var hours []*models.WorkingHours

	// Parse form for each day
	for day := 0; day <= 6; day++ {
		enabled := r.FormValue("day_"+strconv.Itoa(day)+"_enabled") == "on"
		startTime := r.FormValue("day_" + strconv.Itoa(day) + "_start")
		endTime := r.FormValue("day_" + strconv.Itoa(day) + "_end")

		if startTime != "" && endTime != "" {
			hours = append(hours, &models.WorkingHours{
				ID:        uuid.New().String(),
				HostID:    host.Host.ID,
				DayOfWeek: day,
				StartTime: startTime,
				EndTime:   endTime,
				IsEnabled: enabled,
				CreatedAt: now,
				UpdatedAt: now,
			})
		}
	}

	if err := h.handlers.services.Availability.SetWorkingHours(r.Context(), host.Host.ID, hours); err != nil {
		h.handlers.redirect(w, r, "/dashboard/settings?error=update_failed")
		return
	}

	h.handlers.redirect(w, r, "/dashboard/settings?success=hours_updated")
}

func parseIntOrDefault(s string, defaultValue int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return defaultValue
}

// BookingDetails returns booking details as a partial (for modal display)
func (h *DashboardHandler) BookingDetails(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	bookingID := r.PathValue("id")
	booking, err := h.handlers.services.Booking.GetBooking(r.Context(), bookingID)
	if err != nil || booking == nil || booking.HostID != host.Host.ID {
		http.Error(w, "Booking not found", http.StatusNotFound)
		return
	}

	// Get template for meeting name
	template, _ := h.handlers.services.Template.GetTemplate(r.Context(), host.Host.ID, booking.TemplateID)

	h.handlers.renderPartial(w, "booking_details_partial.html", map[string]interface{}{
		"Booking":  booking,
		"Template": template,
	})
}

// AuditLogs renders the audit log viewer page (admin only)
func (h *DashboardHandler) AuditLogs(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	// Admin-only access
	if !host.Host.IsAdmin {
		h.handlers.error(w, r, http.StatusForbidden, "Access denied. Admin privileges required.")
		return
	}

	// Pagination parameters
	page := parseIntOrDefault(r.URL.Query().Get("page"), 1)
	if page < 1 {
		page = 1
	}
	perPage := 25
	offset := (page - 1) * perPage

	// Action filter
	actionFilter := r.URL.Query().Get("action")

	// Get audit logs
	logs, err := h.handlers.services.AuditLog.GetLogs(r.Context(), host.Tenant.ID, perPage, offset)
	if err != nil {
		log.Printf("Error fetching audit logs: %v", err)
		logs = []*models.AuditLog{}
	}

	// Get total count for pagination
	totalCount, err := h.handlers.services.AuditLog.GetLogsCount(r.Context(), host.Tenant.ID)
	if err != nil {
		log.Printf("Error counting audit logs: %v", err)
		totalCount = 0
	}

	totalPages := (totalCount + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}

	h.handlers.render(w, "dashboard_audit_logs.html", PageData{
		Title:        "Audit Logs",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "audit-logs",
		PendingCount: h.getPendingCount(r, host.Host.ID),
		Data: map[string]interface{}{
			"Logs":         logs,
			"Page":         page,
			"TotalPages":   totalPages,
			"TotalCount":   totalCount,
			"ActionFilter": actionFilter,
		},
	})
}

// AgendaDayGroup represents a group of events for a single day
type AgendaDayGroup struct {
	Date   time.Time
	Events []services.AgendaEvent
}

// Agenda renders the agenda view with today's or this week's events
func (h *DashboardHandler) Agenda(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	// Load host timezone
	loc, err := time.LoadLocation(host.Host.Timezone)
	if err != nil {
		log.Printf("Invalid timezone %s, falling back to UTC: %v", host.Host.Timezone, err)
		loc = time.UTC
	}

	// Get current time in host's timezone
	now := time.Now().In(loc)

	// Check view parameter
	view := r.URL.Query().Get("view")
	if view != "week" {
		view = "today"
	}

	var startDate, endDate time.Time
	var dayGroups []AgendaDayGroup

	if view == "week" {
		// This Week: Monday to Sunday of current week
		// Go's time.Weekday: Sunday=0, Monday=1, ..., Saturday=6
		// We want to start on Monday
		weekday := now.Weekday()
		daysFromMonday := int(weekday) - 1
		if weekday == time.Sunday {
			daysFromMonday = 6 // Sunday is the end of the week
		}
		monday := time.Date(now.Year(), now.Month(), now.Day()-daysFromMonday, 0, 0, 0, 0, loc)
		sunday := monday.AddDate(0, 0, 7) // End of Sunday (start of next Monday)

		startDate = monday
		endDate = sunday

		// Fetch events for the week
		events, fetchErr := h.handlers.services.Calendar.GetAgendaEvents(r.Context(), host.Host.ID, startDate, endDate)
		if fetchErr != nil {
			log.Printf("Error fetching agenda events: %v", fetchErr)
			events = []services.AgendaEvent{}
		}

		// Group events by day
		dayGroups = groupEventsByDay(events, monday, loc)
	} else {
		// Today: 00:00 to 23:59:59 in host's timezone
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		endDate = startDate.AddDate(0, 0, 1)
	}

	// Fetch events (for today view or as fallback)
	events, err := h.handlers.services.Calendar.GetAgendaEvents(r.Context(), host.Host.ID, startDate, endDate)
	if err != nil {
		log.Printf("Error fetching agenda events: %v", err)
		events = []services.AgendaEvent{}
	}

	h.handlers.render(w, "dashboard_agenda.html", PageData{
		Title:        "Agenda",
		Host:         host.Host,
		Tenant:       host.Tenant,
		ActiveNav:    "agenda",
		PendingCount: h.getPendingCount(r, host.Host.ID),
		Data: map[string]interface{}{
			"Events":    events,
			"DayGroups": dayGroups,
			"View":      view,
			"Today":     now,
			"Timezone":  host.Host.Timezone,
		},
	})
}

// groupEventsByDay groups events by their date and returns a slice of day groups
func groupEventsByDay(events []services.AgendaEvent, weekStart time.Time, loc *time.Location) []AgendaDayGroup {
	// Create groups for each day of the week
	groups := make([]AgendaDayGroup, 7)
	for i := 0; i < 7; i++ {
		groups[i] = AgendaDayGroup{
			Date:   weekStart.AddDate(0, 0, i),
			Events: []services.AgendaEvent{},
		}
	}

	// Assign events to their respective days
	for _, event := range events {
		eventDate := event.Start.In(loc)
		dayOfWeek := eventDate.Weekday()
		// Convert to Monday=0 index
		dayIndex := int(dayOfWeek) - 1
		if dayOfWeek == time.Sunday {
			dayIndex = 6
		}

		if dayIndex >= 0 && dayIndex < 7 {
			groups[dayIndex].Events = append(groups[dayIndex].Events, event)
		}
	}

	return groups
}

// filterAvailableHosts returns hosts that are not already in the pooled hosts list
func filterAvailableHosts(allHosts []*models.Host, pooledHosts []*models.TemplateHost) []*models.Host {
	if len(pooledHosts) == 0 {
		return allHosts
	}

	pooledMap := make(map[string]bool, len(pooledHosts))
	for _, ph := range pooledHosts {
		pooledMap[ph.HostID] = true
	}

	var available []*models.Host
	for _, h := range allHosts {
		if !pooledMap[h.ID] {
			available = append(available, h)
		}
	}
	return available
}
