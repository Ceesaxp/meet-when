package handlers

import (
	"encoding/json"
	"net/http"
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
	dateStr := r.URL.Query().Get("date")
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

	// Parse date
	var startDate time.Time
	if dateStr != "" {
		startDate, _ = time.Parse("2006-01-02", dateStr)
	}
	if startDate.IsZero() {
		startDate = time.Now()
	}

	// Calculate date range (show one week at a time)
	endDate := startDate.AddDate(0, 0, 7)

	// Parse duration
	duration := 30
	if durationStr != "" {
		for _, d := range template.Durations {
			if durationStr == string(rune(d)) {
				duration = d
				break
			}
		}
	}
	if len(template.Durations) > 0 {
		duration = template.Durations[0]
	}

	// Get available slots
	slots, err := h.handlers.services.Availability.GetAvailableSlots(r.Context(), services.GetAvailableSlotsInput{
		HostID:     host.ID,
		TemplateID: template.ID,
		StartDate:  startDate,
		EndDate:    endDate,
		Duration:   duration,
		Timezone:   timezone,
	})
	if err != nil {
		http.Error(w, "Failed to load availability", http.StatusInternalServerError)
		return
	}

	// Group slots by date
	slotsByDate := make(map[string][]models.TimeSlot)
	for _, slot := range slots {
		dateKey := slot.Start.Format("2006-01-02")
		slotsByDate[dateKey] = append(slotsByDate[dateKey], slot)
	}

	h.handlers.renderPartial(w, "slots_partial.html", map[string]interface{}{
		"Slots":     slotsByDate,
		"StartDate": startDate,
		"EndDate":   endDate,
		"Duration":  duration,
		"Timezone":  timezone,
	})
}

// CreateBooking handles booking form submission
func (h *PublicHandler) CreateBooking(w http.ResponseWriter, r *http.Request) {
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

	duration := 30
	if len(template.Durations) > 0 {
		duration = template.Durations[0]
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
	var answers models.JSONMap
	if template.InviteeQuestions != nil {
		answers = make(models.JSONMap)
		for i, q := range template.InviteeQuestions {
			if qMap, ok := q.(map[string]interface{}); ok {
				if fieldName, ok := qMap["field"].(string); ok {
					answers[fieldName] = r.FormValue("question_" + string(rune(i)))
				}
			}
		}
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

	booking, err := h.handlers.services.Booking.CreateBooking(r.Context(), input)
	if err != nil {
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

	// Redirect to confirmation page
	h.handlers.redirect(w, r, "/booking/"+booking.Booking.Token)
}

// BookingStatus shows the booking confirmation/status page
func (h *PublicHandler) BookingStatus(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	details, err := h.handlers.services.Booking.GetBookingByToken(r.Context(), token)
	if err != nil {
		h.handlers.error(w, r, http.StatusNotFound, "Booking not found")
		return
	}

	h.handlers.render(w, "booking_status.html", PageData{
		Title:   "Booking " + string(details.Booking.Status),
		Host:    details.Host,
		Tenant:  details.Tenant,
		BaseURL: h.handlers.cfg.Server.BaseURL,
		Data: map[string]interface{}{
			"Booking":  details.Booking,
			"Template": details.Template,
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

// Ensure imports are available
type contextKey string

// Import models in handlers
var _ = json.Marshal
