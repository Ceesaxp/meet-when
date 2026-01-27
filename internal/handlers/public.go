package handlers

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/services"
)

// PublicHandler handles public booking pages
type PublicHandler struct {
	handlers *Handlers
}

// HostPage renders the host's public booking page
func (h *PublicHandler) HostPage(w http.ResponseWriter, r *http.Request) {
	tenantSlug := r.PathValue("tenant")
	hostSlug := r.PathValue("host")

	// Get tenant
	tenant, err := h.handlers.services.Auth.GetTenantBySlug(r.Context(), tenantSlug)
	if err != nil || tenant == nil {
		h.handlers.error(w, r, http.StatusNotFound, "Page not found")
		return
	}

	// Get host
	host, err := h.handlers.services.Auth.GetHostBySlug(r.Context(), tenant.ID, hostSlug)
	if err != nil || host == nil {
		h.handlers.error(w, r, http.StatusNotFound, "Page not found")
		return
	}

	// Get active templates
	templates, err := h.handlers.services.Template.GetTemplates(r.Context(), host.ID)
	if err != nil {
		h.handlers.error(w, r, http.StatusInternalServerError, "Failed to load meeting types")
		return
	}

	// Filter to active templates only
	var activeTemplates []*models.MeetingTemplate
	for _, t := range templates {
		if t.IsActive {
			activeTemplates = append(activeTemplates, t)
		}
	}

	h.handlers.render(w, "public_host.html", PageData{
		Title:       host.Name,
		Description: "Book a meeting with " + host.Name,
		Host:        host,
		Tenant:      tenant,
		BaseURL:     h.handlers.cfg.Server.BaseURL,
		Data: map[string]interface{}{
			"Templates": activeTemplates,
		},
	})
}

// TemplatePage renders the booking page for a specific template
func (h *PublicHandler) TemplatePage(w http.ResponseWriter, r *http.Request) {
	tenantSlug := r.PathValue("tenant")
	hostSlug := r.PathValue("host")
	templateSlug := r.PathValue("template")

	// Get tenant
	tenant, err := h.handlers.services.Auth.GetTenantBySlug(r.Context(), tenantSlug)
	if err != nil || tenant == nil {
		h.handlers.error(w, r, http.StatusNotFound, "Page not found")
		return
	}

	// Get host
	host, err := h.handlers.services.Auth.GetHostBySlug(r.Context(), tenant.ID, hostSlug)
	if err != nil || host == nil {
		h.handlers.error(w, r, http.StatusNotFound, "Page not found")
		return
	}

	// Get template
	template, err := h.handlers.services.Template.GetTemplateBySlug(r.Context(), host.ID, templateSlug)
	if err != nil || template == nil || !template.IsActive {
		h.handlers.error(w, r, http.StatusNotFound, "Meeting type not found")
		return
	}

	h.handlers.render(w, "public_template.html", PageData{
		Title:       template.Name + " | " + host.Name,
		Description: template.Description,
		Host:        host,
		Tenant:      tenant,
		BaseURL:     h.handlers.cfg.Server.BaseURL,
		Data: map[string]interface{}{
			"Template": template,
		},
	})
}

// GetSlots returns available time slots (HTMX endpoint)
func (h *PublicHandler) GetSlots(w http.ResponseWriter, r *http.Request) {
	tenantSlug := r.PathValue("tenant")
	hostSlug := r.PathValue("host")
	templateSlug := r.PathValue("template")

	// Parse query parameters
	monthStr := r.URL.Query().Get("month")      // YYYY-MM format for month navigation
	selectedStr := r.URL.Query().Get("selected") // YYYY-MM-DD for selected date
	timezone := r.URL.Query().Get("timezone")
	durationStr := r.URL.Query().Get("duration")

	if timezone == "" {
		timezone = "UTC"
	}

	// Get tenant and host
	tenant, _ := h.handlers.services.Auth.GetTenantBySlug(r.Context(), tenantSlug)
	if tenant == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	host, _ := h.handlers.services.Auth.GetHostBySlug(r.Context(), tenant.ID, hostSlug)
	if host == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	template, _ := h.handlers.services.Template.GetTemplateBySlug(r.Context(), host.ID, templateSlug)
	if template == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Parse month - determines which month to display
	now := time.Now()
	var displayMonth time.Time
	if monthStr != "" {
		if parsed, err := time.Parse("2006-01", monthStr); err == nil {
			displayMonth = parsed
		}
	}
	if displayMonth.IsZero() {
		displayMonth = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	// Parse selected date for showing time slots
	var selectedDate time.Time
	if selectedStr != "" {
		selectedDate, _ = time.Parse("2006-01-02", selectedStr)
	}

	// Calculate date range for fetching slots (full month plus padding for calendar view)
	monthStart := time.Date(displayMonth.Year(), displayMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0) // First day of next month

	// Parse duration - validate against allowed template durations
	duration := 30
	if len(template.Durations) > 0 {
		duration = template.Durations[0] // Default to first duration
	}
	if durationStr != "" {
		if parsedDuration, err := strconv.Atoi(durationStr); err == nil {
			// Validate that the requested duration is allowed for this template
			for _, d := range template.Durations {
				if d == parsedDuration {
					duration = parsedDuration
					break
				}
			}
		}
	}

	// Get available slots for the entire month
	slots, err := h.handlers.services.Availability.GetAvailableSlots(r.Context(), services.GetAvailableSlotsInput{
		HostID:     host.ID,
		TemplateID: template.ID,
		StartDate:  monthStart,
		EndDate:    monthEnd,
		Duration:   duration,
		Timezone:   timezone,
	})
	if err != nil {
		http.Error(w, "Failed to load availability", http.StatusInternalServerError)
		return
	}

	// Group slots by date
	slotsByDate := make(map[string][]models.TimeSlot)
	availableDates := make(map[string]bool)
	for _, slot := range slots {
		dateKey := slot.Start.Format("2006-01-02")
		slotsByDate[dateKey] = append(slotsByDate[dateKey], slot)
		availableDates[dateKey] = true
	}

	// Build calendar grid data
	type CalendarDay struct {
		Date       time.Time
		DayNum     int
		IsInMonth  bool
		IsToday    bool
		IsPast     bool
		IsSelected bool
		HasSlots   bool
	}

	// Get the first day of the month and figure out what weekday it starts on
	firstOfMonth := time.Date(displayMonth.Year(), displayMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	startWeekday := int(firstOfMonth.Weekday()) // 0=Sunday

	// Calculate days in month
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	daysInMonth := lastOfMonth.Day()

	// Build calendar weeks
	var weeks [][]CalendarDay
	var currentWeek []CalendarDay

	// Add empty days before first of month
	for i := 0; i < startWeekday; i++ {
		currentWeek = append(currentWeek, CalendarDay{IsInMonth: false})
	}

	// Add days of month
	today := time.Now().Truncate(24 * time.Hour)
	for day := 1; day <= daysInMonth; day++ {
		date := time.Date(displayMonth.Year(), displayMonth.Month(), day, 0, 0, 0, 0, time.UTC)
		dateKey := date.Format("2006-01-02")

		calDay := CalendarDay{
			Date:       date,
			DayNum:     day,
			IsInMonth:  true,
			IsToday:    date.Year() == today.Year() && date.Month() == today.Month() && date.Day() == today.Day(),
			IsPast:     date.Before(today),
			IsSelected: !selectedDate.IsZero() && date.Year() == selectedDate.Year() && date.Month() == selectedDate.Month() && date.Day() == selectedDate.Day(),
			HasSlots:   availableDates[dateKey],
		}

		currentWeek = append(currentWeek, calDay)

		// Start new week if this is Saturday
		if len(currentWeek) == 7 {
			weeks = append(weeks, currentWeek)
			currentWeek = nil
		}
	}

	// Add empty days after last of month
	for len(currentWeek) > 0 && len(currentWeek) < 7 {
		currentWeek = append(currentWeek, CalendarDay{IsInMonth: false})
	}
	if len(currentWeek) > 0 {
		weeks = append(weeks, currentWeek)
	}

	// Get slots for selected date
	var selectedSlots []models.TimeSlot
	if !selectedDate.IsZero() {
		selectedSlots = slotsByDate[selectedDate.Format("2006-01-02")]
	}

	// Calculate previous and next month
	prevMonth := displayMonth.AddDate(0, -1, 0)
	nextMonth := displayMonth.AddDate(0, 1, 0)

	// Can navigate to previous month if it's current month or later
	canGoPrev := !prevMonth.Before(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC))

	h.handlers.renderPartial(w, "slots_partial.html", map[string]interface{}{
		"CalendarWeeks":   weeks,
		"MonthDisplay":    displayMonth.Format("January 2006"),
		"MonthValue":      displayMonth.Format("2006-01"),
		"PrevMonth":       prevMonth.Format("2006-01"),
		"NextMonth":       nextMonth.Format("2006-01"),
		"CanGoPrev":       canGoPrev,
		"SelectedDate":    selectedDate,
		"SelectedDisplay": selectedDate.Format("Monday, January 2"),
		"SelectedSlots":   selectedSlots,
		"Duration":        duration,
		"Timezone":        timezone,
	})
}

// CreateBooking handles booking form submission
func (h *PublicHandler) CreateBooking(w http.ResponseWriter, r *http.Request) {
	log.Printf("[BOOKING] CreateBooking called: method=%s path=%s", r.Method, r.URL.Path)

	tenantSlug := r.PathValue("tenant")
	hostSlug := r.PathValue("host")
	templateSlug := r.PathValue("template")

	if err := r.ParseForm(); err != nil {
		h.handlers.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	// Get tenant and host
	tenant, _ := h.handlers.services.Auth.GetTenantBySlug(r.Context(), tenantSlug)
	if tenant == nil {
		h.handlers.error(w, r, http.StatusNotFound, "Page not found")
		return
	}

	host, _ := h.handlers.services.Auth.GetHostBySlug(r.Context(), tenant.ID, hostSlug)
	if host == nil {
		h.handlers.error(w, r, http.StatusNotFound, "Page not found")
		return
	}

	template, _ := h.handlers.services.Template.GetTemplateBySlug(r.Context(), host.ID, templateSlug)
	if template == nil || !template.IsActive {
		h.handlers.error(w, r, http.StatusNotFound, "Meeting type not found")
		return
	}

	// Parse form data
	startTimeStr := r.FormValue("start_time")
	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		h.handlers.error(w, r, http.StatusBadRequest, "Invalid time format")
		return
	}

	// Parse duration from form, validate against template durations
	duration := 30
	if len(template.Durations) > 0 {
		duration = template.Durations[0] // Default to first duration
	}
	if durationStr := r.FormValue("duration"); durationStr != "" {
		if parsedDuration, err := strconv.Atoi(durationStr); err == nil {
			// Validate that the requested duration is allowed for this template
			for _, d := range template.Durations {
				if d == parsedDuration {
					duration = parsedDuration
					break
				}
			}
		}
	}

	// Parse additional guests
	var additionalGuests []string
	guestsStr := r.FormValue("additional_guests")
	if guestsStr != "" {
		// Split by comma or newline
		for _, g := range splitEmails(guestsStr) {
			if g != "" {
				additionalGuests = append(additionalGuests, g)
			}
		}
	}

	// Parse custom answers
	answers := make(models.JSONMap)
	if template.InviteeQuestions != nil {
		for i, q := range template.InviteeQuestions {
			if qMap, ok := q.(map[string]interface{}); ok {
				if fieldName, ok := qMap["field"].(string); ok {
					answers[fieldName] = r.FormValue("question_" + strconv.Itoa(i))
				}
			}
		}
	}

	// Store agenda/subject if provided
	if agenda := r.FormValue("agenda"); agenda != "" {
		answers["agenda"] = agenda
	}

	input := services.CreateBookingInput{
		TemplateID:       template.ID,
		HostID:           host.ID,
		TenantID:         tenant.ID,
		StartTime:        startTime,
		Duration:         duration,
		InviteeName:      r.FormValue("name"),
		InviteeEmail:     r.FormValue("email"),
		InviteeTimezone:  r.FormValue("timezone"),
		InviteePhone:     r.FormValue("phone"),
		AdditionalGuests: additionalGuests,
		Answers:          answers,
	}

	log.Printf("[BOOKING] Creating booking: template=%s invitee=%s time=%s", input.TemplateID, input.InviteeEmail, input.StartTime)

	booking, err := h.handlers.services.Booking.CreateBooking(r.Context(), input)
	if err != nil {
		log.Printf("[BOOKING] Error creating booking: %v", err)
		message := "Failed to create booking"
		switch err {
		case services.ErrSlotNotAvailable:
			message = "This time slot is no longer available"
		case services.ErrInvalidBookingTime:
			message = "Invalid booking time"
		}
		h.handlers.render(w, "public_template.html", PageData{
			Title:  template.Name + " | " + host.Name,
			Host:   host,
			Tenant: tenant,
			Flash:  &FlashMessage{Type: "error", Message: message},
			Data: map[string]interface{}{
				"Template": template,
			},
		})
		return
	}

	log.Printf("[BOOKING] Booking created successfully: id=%s token=%s", booking.Booking.ID, booking.Booking.Token)

	// Redirect to confirmation page
	redirectURL := "/booking/" + booking.Booking.Token
	log.Printf("[BOOKING] Redirecting to: %s", redirectURL)
	h.handlers.redirect(w, r, redirectURL)
}

// BookingStatus shows the booking confirmation/status page
func (h *PublicHandler) BookingStatus(w http.ResponseWriter, r *http.Request) {
	log.Printf("[BOOKING-STATUS] Handler called: path=%s", r.URL.Path)
	token := r.PathValue("token")
	log.Printf("[BOOKING-STATUS] Token from path: %s", token)

	details, err := h.handlers.services.Booking.GetBookingByToken(r.Context(), token)
	if err != nil {
		log.Printf("[BOOKING-STATUS] Error getting booking: %v", err)
		h.handlers.error(w, r, http.StatusNotFound, "Booking not found")
		return
	}
	log.Printf("[BOOKING-STATUS] Booking found: id=%s status=%s", details.Booking.ID, details.Booking.Status)

	// Check for action notifications from query params
	rescheduled := r.URL.Query().Get("rescheduled") == "true"
	cancelled := r.URL.Query().Get("cancelled") == "true"

	h.handlers.render(w, "booking_status.html", PageData{
		Title:   "Booking " + string(details.Booking.Status),
		Host:    details.Host,
		Tenant:  details.Tenant,
		BaseURL: h.handlers.cfg.Server.BaseURL,
		Data: map[string]interface{}{
			"Booking":     details.Booking,
			"Template":    details.Template,
			"Rescheduled": rescheduled,
			"Cancelled":   cancelled,
		},
	})
}

// CancelBooking handles booking cancellation by invitee
func (h *PublicHandler) CancelBooking(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	if err := r.ParseForm(); err != nil {
		h.handlers.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	reason := r.FormValue("reason")

	err := h.handlers.services.Booking.CancelBookingByToken(r.Context(), token, reason)
	if err != nil {
		h.handlers.error(w, r, http.StatusBadRequest, "Failed to cancel booking")
		return
	}

	h.handlers.redirect(w, r, "/booking/"+token+"?cancelled=true")
}

// DownloadICS serves an ICS calendar file for a booking
func (h *PublicHandler) DownloadICS(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	details, err := h.handlers.services.Booking.GetBookingByToken(r.Context(), token)
	if err != nil {
		h.handlers.error(w, r, http.StatusNotFound, "Booking not found")
		return
	}

	// Only allow ICS download for confirmed or pending bookings
	if details.Booking.Status != models.BookingStatusConfirmed && details.Booking.Status != models.BookingStatusPending {
		h.handlers.error(w, r, http.StatusBadRequest, "Cannot download calendar for this booking")
		return
	}

	// Generate ICS content
	ics := h.handlers.services.Email.GenerateICS(details)

	// Set headers for ICS file download
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"meeting.ics\"")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(ics)); err != nil {
		log.Printf("Error writing ICS response: %v", err)
	}
}

// ReschedulePage shows the reschedule page
func (h *PublicHandler) ReschedulePage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	details, err := h.handlers.services.Booking.GetBookingByToken(r.Context(), token)
	if err != nil {
		h.handlers.error(w, r, http.StatusNotFound, "Booking not found")
		return
	}

	if details.Booking.Status == models.BookingStatusCancelled {
		h.handlers.error(w, r, http.StatusBadRequest, "This booking has been cancelled")
		return
	}

	if details.Booking.Status == models.BookingStatusRejected {
		h.handlers.error(w, r, http.StatusBadRequest, "This booking has been rejected")
		return
	}

	h.handlers.render(w, "reschedule.html", PageData{
		Title:   "Reschedule Booking",
		Host:    details.Host,
		Tenant:  details.Tenant,
		BaseURL: h.handlers.cfg.Server.BaseURL,
		Data: map[string]interface{}{
			"Booking":  details.Booking,
			"Template": details.Template,
		},
	})
}

// GetRescheduleSlots returns available time slots for rescheduling (HTMX endpoint)
func (h *PublicHandler) GetRescheduleSlots(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	// Get booking details
	details, err := h.handlers.services.Booking.GetBookingByToken(r.Context(), token)
	if err != nil {
		http.Error(w, "Booking not found", http.StatusNotFound)
		return
	}

	// Parse query parameters
	monthStr := r.URL.Query().Get("month")       // YYYY-MM format for month navigation
	selectedStr := r.URL.Query().Get("selected") // YYYY-MM-DD for selected date
	timezone := r.URL.Query().Get("timezone")
	durationStr := r.URL.Query().Get("duration")

	if timezone == "" {
		timezone = details.Booking.InviteeTimezone
	}
	if timezone == "" {
		timezone = "UTC"
	}

	// Parse month - determines which month to display
	now := time.Now()
	var displayMonth time.Time
	if monthStr != "" {
		if parsed, err := time.Parse("2006-01", monthStr); err == nil {
			displayMonth = parsed
		}
	}
	if displayMonth.IsZero() {
		displayMonth = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	// Parse selected date for showing time slots
	var selectedDate time.Time
	if selectedStr != "" {
		selectedDate, _ = time.Parse("2006-01-02", selectedStr)
	}

	// Calculate date range for fetching slots (full month)
	monthStart := time.Date(displayMonth.Year(), displayMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)

	// Parse duration - validate against allowed template durations
	duration := details.Booking.Duration // Default to current booking duration
	if durationStr != "" {
		if parsedDuration, err := strconv.Atoi(durationStr); err == nil {
			// Validate that the requested duration is allowed for this template
			for _, d := range details.Template.Durations {
				if d == parsedDuration {
					duration = parsedDuration
					break
				}
			}
		}
	}

	// Get available slots for the entire month
	slots, err := h.handlers.services.Availability.GetAvailableSlots(r.Context(), services.GetAvailableSlotsInput{
		HostID:     details.Host.ID,
		TemplateID: details.Template.ID,
		StartDate:  monthStart,
		EndDate:    monthEnd,
		Duration:   duration,
		Timezone:   timezone,
	})
	if err != nil {
		http.Error(w, "Failed to load availability", http.StatusInternalServerError)
		return
	}

	// Group slots by date
	slotsByDate := make(map[string][]models.TimeSlot)
	availableDates := make(map[string]bool)
	for _, slot := range slots {
		dateKey := slot.Start.Format("2006-01-02")
		slotsByDate[dateKey] = append(slotsByDate[dateKey], slot)
		availableDates[dateKey] = true
	}

	// Build calendar grid data
	type CalendarDay struct {
		Date       time.Time
		DayNum     int
		IsInMonth  bool
		IsToday    bool
		IsPast     bool
		IsSelected bool
		HasSlots   bool
	}

	// Get the first day of the month and figure out what weekday it starts on
	firstOfMonth := time.Date(displayMonth.Year(), displayMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	startWeekday := int(firstOfMonth.Weekday())

	// Calculate days in month
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	daysInMonth := lastOfMonth.Day()

	// Build calendar weeks
	var weeks [][]CalendarDay
	var currentWeek []CalendarDay

	// Add empty days before first of month
	for i := 0; i < startWeekday; i++ {
		currentWeek = append(currentWeek, CalendarDay{IsInMonth: false})
	}

	// Add days of month
	today := time.Now().Truncate(24 * time.Hour)
	for day := 1; day <= daysInMonth; day++ {
		date := time.Date(displayMonth.Year(), displayMonth.Month(), day, 0, 0, 0, 0, time.UTC)
		dateKey := date.Format("2006-01-02")

		calDay := CalendarDay{
			Date:       date,
			DayNum:     day,
			IsInMonth:  true,
			IsToday:    date.Year() == today.Year() && date.Month() == today.Month() && date.Day() == today.Day(),
			IsPast:     date.Before(today),
			IsSelected: !selectedDate.IsZero() && date.Year() == selectedDate.Year() && date.Month() == selectedDate.Month() && date.Day() == selectedDate.Day(),
			HasSlots:   availableDates[dateKey],
		}

		currentWeek = append(currentWeek, calDay)

		if len(currentWeek) == 7 {
			weeks = append(weeks, currentWeek)
			currentWeek = nil
		}
	}

	// Add empty days after last of month
	for len(currentWeek) > 0 && len(currentWeek) < 7 {
		currentWeek = append(currentWeek, CalendarDay{IsInMonth: false})
	}
	if len(currentWeek) > 0 {
		weeks = append(weeks, currentWeek)
	}

	// Get slots for selected date
	var selectedSlots []models.TimeSlot
	if !selectedDate.IsZero() {
		selectedSlots = slotsByDate[selectedDate.Format("2006-01-02")]
	}

	// Calculate previous and next month
	prevMonth := displayMonth.AddDate(0, -1, 0)
	nextMonth := displayMonth.AddDate(0, 1, 0)
	canGoPrev := !prevMonth.Before(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC))

	h.handlers.renderPartial(w, "reschedule_slots_partial.html", map[string]interface{}{
		"CalendarWeeks":   weeks,
		"MonthDisplay":    displayMonth.Format("January 2006"),
		"MonthValue":      displayMonth.Format("2006-01"),
		"PrevMonth":       prevMonth.Format("2006-01"),
		"NextMonth":       nextMonth.Format("2006-01"),
		"CanGoPrev":       canGoPrev,
		"SelectedDate":    selectedDate,
		"SelectedDisplay": selectedDate.Format("Monday, January 2"),
		"SelectedSlots":   selectedSlots,
		"Duration":        duration,
		"Timezone":        timezone,
		"Token":           token,
	})
}

// RescheduleBooking handles reschedule form submission
func (h *PublicHandler) RescheduleBooking(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	if err := r.ParseForm(); err != nil {
		h.handlers.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	// Get booking details for error display
	details, err := h.handlers.services.Booking.GetBookingByToken(r.Context(), token)
	if err != nil {
		h.handlers.error(w, r, http.StatusNotFound, "Booking not found")
		return
	}

	// Parse form data
	startTimeStr := r.FormValue("start_time")
	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		h.handlers.render(w, "reschedule.html", PageData{
			Title:   "Reschedule Booking",
			Host:    details.Host,
			Tenant:  details.Tenant,
			BaseURL: h.handlers.cfg.Server.BaseURL,
			Flash:   &FlashMessage{Type: "error", Message: "Invalid time format"},
			Data: map[string]interface{}{
				"Booking":  details.Booking,
				"Template": details.Template,
			},
		})
		return
	}

	// Parse duration
	duration := details.Booking.Duration
	if durationStr := r.FormValue("duration"); durationStr != "" {
		if parsedDuration, err := strconv.Atoi(durationStr); err == nil {
			// Validate against template durations
			for _, d := range details.Template.Durations {
				if d == parsedDuration {
					duration = parsedDuration
					break
				}
			}
		}
	}

	log.Printf("[RESCHEDULE] Rescheduling booking: token=%s new_time=%s duration=%d", token, startTime, duration)

	// Reschedule the booking
	newDetails, oldStartTime, err := h.handlers.services.Booking.RescheduleBooking(r.Context(), services.RescheduleBookingInput{
		Token:        token,
		NewStartTime: startTime,
		NewDuration:  duration,
	})
	if err != nil {
		log.Printf("[RESCHEDULE] Error rescheduling booking: %v", err)
		message := "Failed to reschedule booking"
		switch err {
		case services.ErrSlotNotAvailable:
			message = "This time slot is no longer available"
		case services.ErrInvalidBookingTime:
			message = "Invalid booking time - please select a time further in the future"
		case services.ErrBookingCancelled:
			message = "This booking has been cancelled and cannot be rescheduled"
		}
		h.handlers.render(w, "reschedule.html", PageData{
			Title:   "Reschedule Booking",
			Host:    details.Host,
			Tenant:  details.Tenant,
			BaseURL: h.handlers.cfg.Server.BaseURL,
			Flash:   &FlashMessage{Type: "error", Message: message},
			Data: map[string]interface{}{
				"Booking":  details.Booking,
				"Template": details.Template,
			},
		})
		return
	}

	log.Printf("[RESCHEDULE] Booking rescheduled successfully: id=%s old_time=%s new_time=%s",
		newDetails.Booking.ID, oldStartTime, newDetails.Booking.StartTime)

	// Send reschedule notification emails
	h.handlers.services.Email.SendBookingRescheduled(r.Context(), newDetails, oldStartTime)

	// Redirect to booking status page with success message
	h.handlers.redirect(w, r, "/booking/"+token+"?rescheduled=true")
}

// splitEmails splits a string of emails by comma, semicolon, or newline
func splitEmails(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' || c == ';' || c == '\n' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else if c != ' ' && c != '\r' {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
