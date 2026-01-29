package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/services"
)

// AuthHandler handles authentication routes
type AuthHandler struct {
	handlers *Handlers
}

// LoginPage renders the login page
func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.handlers.render(w, "login.html", PageData{
		Title: "Login",
	})
}

// Login handles login form submission using simplified login (email + password only)
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.handlers.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	input := services.SimplifiedLoginInput{
		Email:    r.FormValue("email"),
		Password: r.FormValue("password"),
	}

	result, err := h.handlers.services.Auth.SimplifiedLogin(r.Context(), input)
	if err != nil {
		h.handlers.render(w, "login.html", PageData{
			Title: "Login",
			Flash: &FlashMessage{Type: "error", Message: "Invalid email or password"},
			Data:  map[string]string{"email": input.Email},
		})
		return
	}

	// Handle multi-org case: render org selection page
	if result.RequiresOrgSelection {
		h.handlers.render(w, "login_select_org.html", PageData{
			Title: "Select Organization",
			Data: map[string]interface{}{
				"AvailableOrgs": result.AvailableOrgs,
			},
		})
		return
	}

	// Single-org case: create session and redirect to dashboard
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    result.SessionToken,
		Path:     "/",
		MaxAge:   int(24 * time.Hour / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	h.handlers.redirect(w, r, "/dashboard")
}

// RegisterPage renders the registration page
func (h *AuthHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	// Preserve ref parameter from query string (e.g., from signup tracking)
	ref := r.URL.Query().Get("ref")

	h.handlers.render(w, "register.html", PageData{
		Title: "Create Account",
		Data: map[string]interface{}{
			"ref": ref,
		},
	})
}

// Register handles registration form submission
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.handlers.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	input := services.RegisterInput{
		TenantName: r.FormValue("tenant_name"),
		TenantSlug: r.FormValue("tenant_slug"),
		Name:       r.FormValue("name"),
		Email:      r.FormValue("email"),
		Password:   r.FormValue("password"),
		Timezone:   r.FormValue("timezone"),
	}

	result, err := h.handlers.services.Auth.Register(r.Context(), input)
	if err != nil {
		message := "Registration failed"
		switch err {
		case services.ErrEmailExists:
			message = "Email already registered"
		case services.ErrTenantExists:
			message = "Organization name already taken"
		case services.ErrInvalidEmail:
			message = "Invalid email format"
		case services.ErrWeakPassword:
			message = "Password must be at least 8 characters"
		}

		// Preserve ref parameter on error
		ref := r.FormValue("ref")

		h.handlers.render(w, "register.html", PageData{
			Title: "Create Account",
			Flash: &FlashMessage{Type: "error", Message: message},
			Data: map[string]interface{}{
				"tenant_name": input.TenantName,
				"tenant_slug": input.TenantSlug,
				"name":        input.Name,
				"email":       input.Email,
				"timezone":    input.Timezone,
				"ref":         ref,
			},
		})
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    result.SessionToken,
		Path:     "/",
		MaxAge:   int(24 * time.Hour / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Track signup conversion if ref parameter indicates booking source
	ref := r.FormValue("ref")
	if ref != "" && len(ref) > 8 && ref[:8] == "booking:" {
		// Mark the most recent unregistered conversion for this email as registered
		// Ignore errors - don't block registration flow
		_ = h.handlers.repos.SignupConversion.MarkRegistered(r.Context(), input.Email)
	}

	// Redirect new users to onboarding
	h.handlers.redirect(w, r, "/onboarding/step/1")
}

// Logout handles logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		_ = h.handlers.services.Session.DeleteSession(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	h.handlers.redirect(w, r, "/auth/login")
}

// GoogleCallback handles Google OAuth callback
func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" {
		h.handlers.error(w, r, http.StatusBadRequest, "Missing authorization code")
		return
	}

	// State contains hostID for calendar connection
	// For login/register, it would be different
	if state != "" {
		// Check if this is from onboarding (state ends with :onboarding)
		fromOnboarding := false
		hostID := state
		if len(state) > 11 && state[len(state)-11:] == ":onboarding" {
			fromOnboarding = true
			hostID = state[:len(state)-11]
		}

		// This is a calendar connection callback
		// The state should contain the host ID
		_, err := h.handlers.services.Calendar.ConnectGoogleCalendar(r.Context(), services.GoogleCalendarConnectInput{
			HostID:      hostID,
			AuthCode:    code,
			RedirectURI: h.handlers.cfg.OAuth.Google.RedirectURL,
		})
		if err != nil {
			if fromOnboarding {
				h.handlers.redirect(w, r, "/onboarding/step/2?error=connection_failed")
			} else {
				h.handlers.redirect(w, r, "/dashboard/calendars?error=connection_failed")
			}
			return
		}
		if fromOnboarding {
			h.handlers.redirect(w, r, "/onboarding/step/3")
		} else {
			h.handlers.redirect(w, r, "/dashboard/calendars?success=calendar_connected")
		}
		return
	}

	h.handlers.error(w, r, http.StatusBadRequest, "Invalid state")
}

// ZoomCallback handles Zoom OAuth callback
func (h *AuthHandler) ZoomCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" {
		h.handlers.error(w, r, http.StatusBadRequest, "Missing authorization code")
		return
	}

	if state != "" {
		_, err := h.handlers.services.Conferencing.ConnectZoom(r.Context(), state, code, h.handlers.cfg.OAuth.Zoom.RedirectURL)
		if err != nil {
			h.handlers.redirect(w, r, "/dashboard/calendars?error=zoom_connection_failed")
			return
		}
		h.handlers.redirect(w, r, "/dashboard/calendars?success=zoom_connected")
		return
	}

	h.handlers.error(w, r, http.StatusBadRequest, "Invalid state")
}

// SelectOrg handles organization selection for multi-org users
func (h *AuthHandler) SelectOrg(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.handlers.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	hostID := r.FormValue("host_id")
	selectionToken := r.FormValue("selection_token")

	if hostID == "" || selectionToken == "" {
		h.handlers.redirect(w, r, "/auth/login?error=invalid_selection")
		return
	}

	input := services.CompleteOrgSelectionInput{
		HostID:         hostID,
		SelectionToken: selectionToken,
	}

	_, sessionToken, err := h.handlers.services.Auth.CompleteOrgSelection(r.Context(), input)
	if err != nil {
		// Redirect to login with error message for invalid/expired token
		h.handlers.redirect(w, r, "/auth/login?error=session_expired")
		return
	}

	// Set session cookie and redirect to dashboard
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   int(24 * time.Hour / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	h.handlers.redirect(w, r, "/dashboard")
}

// TrackSignupCTA tracks when an invitee clicks the signup CTA from booking confirmation
func (h *AuthHandler) TrackSignupCTA(w http.ResponseWriter, r *http.Request) {
	// Get ref parameter
	ref := r.URL.Query().Get("ref")

	// Parse ref format: "booking:{token}"
	if ref != "" && len(ref) > 8 && ref[:8] == "booking:" {
		token := ref[8:]

		// Look up booking by token
		booking, err := h.handlers.services.Booking.GetBookingByToken(r.Context(), token)
		if err == nil && booking != nil {
			// Get tenant ID from the booking's host
			host, err := h.handlers.repos.Host.GetByID(r.Context(), booking.Booking.HostID)
			if err == nil && host != nil {
				// Create signup conversion record
				conversion := &models.SignupConversion{
					ID:              uuid.New().String(),
					SourceBookingID: &booking.Booking.ID,
					InviteeEmail:    booking.Booking.InviteeEmail,
					ClickedAt:       models.Now(),
					RegisteredAt:    nil,
					TenantID:        host.TenantID,
					CreatedAt:       models.Now(),
					UpdatedAt:       models.Now(),
				}

				// Log the conversion (ignore errors - don't block redirect)
				_ = h.handlers.repos.SignupConversion.Create(r.Context(), conversion)
			}
		}
	}

	// Always redirect to register page with ref parameter preserved
	if ref != "" {
		h.handlers.redirect(w, r, "/auth/register?ref="+ref)
	} else {
		h.handlers.redirect(w, r, "/auth/register")
	}
}
