package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/middleware"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/services"
)

// DashboardHandler handles dashboard routes
type DashboardHandler struct {
	handlers *Handlers
}

// Home renders the dashboard home page
func (h *DashboardHandler) Home(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	// Get upcoming bookings
	bookings, _ := h.handlers.services.Booking.GetBookings(r.Context(), host.Host.ID, nil)

	// Get pending bookings count
	pending, _ := h.handlers.services.Booking.GetPendingBookings(r.Context(), host.Host.ID)

	// Get templates count
	templates, _ := h.handlers.services.Template.GetTemplates(r.Context(), host.Host.ID)

	h.handlers.render(w, "dashboard_home.html", PageData{
		Title:  "Dashboard",
		Host:   host.Host,
		Tenant: host.Tenant,
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
		Title:  "Calendars & Integrations",
		Host:   host.Host,
		Tenant: host.Tenant,
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

	h.handlers.render(w, "dashboard_templates.html", PageData{
		Title:  "Meeting Templates",
		Host:   host.Host,
		Tenant: host.Tenant,
		Data: map[string]interface{}{
			"Templates": templates,
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
		Title:  "New Meeting Template",
		Host:   host.Host,
		Tenant: host.Tenant,
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
	}

	_, err := h.handlers.services.Template.CreateTemplate(r.Context(), input)
	if err != nil {
		calendars, _ := h.handlers.services.Calendar.GetCalendars(r.Context(), host.Host.ID)
		h.handlers.render(w, "dashboard_template_form.html", PageData{
			Title:  "New Meeting Template",
			Host:   host.Host,
			Tenant: host.Tenant,
			Flash:  &FlashMessage{Type: "error", Message: "Failed to create template: " + err.Error()},
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

	h.handlers.render(w, "dashboard_template_form.html", PageData{
		Title:  "Edit Template",
		Host:   host.Host,
		Tenant: host.Tenant,
		Data: map[string]interface{}{
			"Template":  template,
			"Calendars": calendars,
			"IsNew":     false,
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
		IsActive:          r.FormValue("is_active") == "on",
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

	bookings, _ := h.handlers.services.Booking.GetBookings(r.Context(), host.Host.ID, status)
	templates, _ := h.handlers.services.Template.GetTemplates(r.Context(), host.Host.ID)

	// Create template map for display
	templateMap := make(map[string]*models.MeetingTemplate)
	for _, t := range templates {
		templateMap[t.ID] = t
	}

	h.handlers.render(w, "dashboard_bookings.html", PageData{
		Title:  "Bookings",
		Host:   host.Host,
		Tenant: host.Tenant,
		Data: map[string]interface{}{
			"Bookings":  bookings,
			"Templates": templateMap,
			"Filter":    filter,
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

	h.handlers.render(w, "dashboard_settings.html", PageData{
		Title:  "Settings",
		Host:   host.Host,
		Tenant: host.Tenant,
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

// UpdateWorkingHours updates working hours
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
