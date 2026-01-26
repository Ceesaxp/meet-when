package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/middleware"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/services"
)

// OnboardingHandler handles onboarding routes
type OnboardingHandler struct {
	handlers *Handlers
}

// Step displays the onboarding step page
func (h *OnboardingHandler) Step(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	// If onboarding is already completed, redirect to dashboard
	if host.Host.OnboardingCompleted {
		h.handlers.redirect(w, r, "/dashboard")
		return
	}

	stepStr := r.PathValue("step")
	step, err := strconv.Atoi(stepStr)
	if err != nil || step < 1 || step > 3 {
		step = 1
	}

	// Determine completed status for each step
	workingHours, _ := h.handlers.services.Availability.GetWorkingHours(r.Context(), host.Host.ID)
	calendars, _ := h.handlers.services.Calendar.GetCalendars(r.Context(), host.Host.ID)
	templates, _ := h.handlers.services.Template.GetTemplates(r.Context(), host.Host.ID)

	step1Complete := len(workingHours) > 0
	step2Complete := len(calendars) > 0
	step3Complete := len(templates) > 0

	var data interface{}
	switch step {
	case 1:
		// Working hours setup
		hoursByDay := make(map[int][]*models.WorkingHours)
		for _, wh := range workingHours {
			hoursByDay[wh.DayOfWeek] = append(hoursByDay[wh.DayOfWeek], wh)
		}
		data = map[string]interface{}{
			"WorkingHours":  hoursByDay,
			"DayNames":      []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
			"Step1Complete": step1Complete,
			"Step2Complete": step2Complete,
			"Step3Complete": step3Complete,
		}
	case 2:
		// Calendar connection
		googleAuthURL := h.handlers.services.Calendar.GetGoogleAuthURL(host.Host.ID)
		data = map[string]interface{}{
			"Calendars":     calendars,
			"GoogleAuthURL": googleAuthURL,
			"Step1Complete": step1Complete,
			"Step2Complete": step2Complete,
			"Step3Complete": step3Complete,
		}
	case 3:
		// Create first template
		data = map[string]interface{}{
			"Calendars":     calendars,
			"Step1Complete": step1Complete,
			"Step2Complete": step2Complete,
			"Step3Complete": step3Complete,
		}
	}

	h.handlers.render(w, "onboarding.html", PageData{
		Title:  "Welcome to MeetWhen",
		Host:   host.Host,
		Tenant: host.Tenant,
		Data: map[string]interface{}{
			"Step":          step,
			"StepData":      data,
			"Step1Complete": step1Complete,
			"Step2Complete": step2Complete,
			"Step3Complete": step3Complete,
		},
	})
}

// SaveWorkingHours saves working hours from onboarding step 1
func (h *OnboardingHandler) SaveWorkingHours(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/onboarding/step/1?error=invalid_form")
		return
	}

	// Save timezone if provided
	timezone := r.FormValue("timezone")
	if timezone != "" {
		host.Host.Timezone = timezone
		if err := h.handlers.services.Auth.UpdateHost(r.Context(), host.Host); err != nil {
			log.Printf("[ONBOARDING] Failed to update host timezone: %v", err)
			// Continue anyway, timezone is not critical
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
		h.handlers.redirect(w, r, "/onboarding/step/1?error=save_failed")
		return
	}

	h.handlers.redirect(w, r, "/onboarding/step/2")
}

// ConnectGoogleCalendar initiates Google Calendar OAuth from onboarding
func (h *OnboardingHandler) ConnectGoogleCalendar(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	// Use state to indicate this is from onboarding
	authURL := h.handlers.services.Calendar.GetGoogleAuthURL(host.Host.ID + ":onboarding")
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

// ConnectCalDAV handles CalDAV/iCloud connection from onboarding
func (h *OnboardingHandler) ConnectCalDAV(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/onboarding/step/2?error=invalid_form")
		return
	}

	provider := r.FormValue("provider")
	if provider == "" {
		provider = "caldav"
	}

	input := services.CalDAVConnectInput{
		HostID:   host.Host.ID,
		Name:     r.FormValue("name"),
		URL:      r.FormValue("url"),
		Username: r.FormValue("username"),
		Password: r.FormValue("password"),
		Provider: models.CalendarProvider(provider),
	}

	if _, err := h.handlers.services.Calendar.ConnectCalDAV(r.Context(), input); err != nil {
		log.Printf("[ONBOARDING] CalDAV connection error: %v", err)
		h.handlers.redirect(w, r, "/onboarding/step/2?error=connection_failed")
		return
	}

	h.handlers.redirect(w, r, "/onboarding/step/3")
}

// SkipStep skips the current onboarding step
func (h *OnboardingHandler) SkipStep(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	stepStr := r.PathValue("step")
	step, err := strconv.Atoi(stepStr)
	if err != nil {
		step = 1
	}

	// Move to next step or complete
	nextStep := step + 1
	if nextStep > 3 {
		// Mark onboarding as complete and go to dashboard
		_ = h.handlers.services.Auth.CompleteOnboarding(r.Context(), host.Host.ID)
		h.handlers.redirect(w, r, "/dashboard")
		return
	}

	h.handlers.redirect(w, r, "/onboarding/step/"+strconv.Itoa(nextStep))
}

// CreateTemplate creates a meeting template from onboarding step 3
func (h *OnboardingHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.handlers.redirect(w, r, "/onboarding/step/3?error=invalid_form")
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

	// Parse invitee questions (if any)
	var inviteeQuestions models.JSONArray
	if questionsJSON := r.FormValue("invitee_questions"); questionsJSON != "" {
		if err := json.Unmarshal([]byte(questionsJSON), &inviteeQuestions); err != nil {
			log.Printf("[ONBOARDING] Failed to parse invitee_questions: %v", err)
		}
	}

	input := services.CreateTemplateInput{
		HostID:           host.Host.ID,
		TenantID:         host.Tenant.ID,
		Slug:             r.FormValue("slug"),
		Name:             r.FormValue("name"),
		Description:      r.FormValue("description"),
		Durations:        durations,
		LocationType:     models.ConferencingProvider(r.FormValue("location_type")),
		CustomLocation:   r.FormValue("custom_location"),
		CalendarID:       r.FormValue("calendar_id"),
		RequiresApproval: r.FormValue("requires_approval") == "on",
		MinNoticeMinutes: parseIntOrDefault(r.FormValue("min_notice_minutes"), 60),
		MaxScheduleDays:  parseIntOrDefault(r.FormValue("max_schedule_days"), 14),
		InviteeQuestions: inviteeQuestions,
	}

	_, err := h.handlers.services.Template.CreateTemplate(r.Context(), input)
	if err != nil {
		calendars, _ := h.handlers.services.Calendar.GetCalendars(r.Context(), host.Host.ID)
		h.handlers.render(w, "onboarding.html", PageData{
			Title:  "Welcome to MeetWhen",
			Host:   host.Host,
			Tenant: host.Tenant,
			Flash:  &FlashMessage{Type: "error", Message: "Failed to create template: " + err.Error()},
			Data: map[string]interface{}{
				"Step": 3,
				"StepData": map[string]interface{}{
					"Calendars": calendars,
				},
			},
		})
		return
	}

	// Mark onboarding as complete
	_ = h.handlers.services.Auth.CompleteOnboarding(r.Context(), host.Host.ID)

	h.handlers.redirect(w, r, "/onboarding/complete")
}

// Complete shows the onboarding completion page
func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		h.handlers.redirect(w, r, "/auth/login")
		return
	}

	// Get templates for the booking link
	templates, _ := h.handlers.services.Template.GetTemplates(r.Context(), host.Host.ID)

	// Build public booking URL
	baseURL := h.handlers.cfg.Server.BaseURL
	bookingURL := baseURL + "/m/" + host.Tenant.Slug + "/" + host.Host.Slug

	h.handlers.render(w, "onboarding_complete.html", PageData{
		Title:  "You're All Set!",
		Host:   host.Host,
		Tenant: host.Tenant,
		Data: map[string]interface{}{
			"BookingURL": bookingURL,
			"Templates":  templates,
		},
	})
}
