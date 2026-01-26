package handlers

import (
	"net/http"
	"time"

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

// Login handles login form submission
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.handlers.error(w, r, http.StatusBadRequest, "Invalid form data")
		return
	}

	input := services.LoginInput{
		TenantSlug: r.FormValue("tenant"),
		Email:      r.FormValue("email"),
		Password:   r.FormValue("password"),
	}

	_, sessionToken, err := h.handlers.services.Auth.Login(r.Context(), input)
	if err != nil {
		h.handlers.render(w, "login.html", PageData{
			Title: "Login",
			Flash: &FlashMessage{Type: "error", Message: "Invalid email or password"},
			Data:  map[string]string{"tenant": input.TenantSlug, "email": input.Email},
		})
		return
	}

	// Set session cookie
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

// RegisterPage renders the registration page
func (h *AuthHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	h.handlers.render(w, "register.html", PageData{
		Title: "Create Account",
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

		h.handlers.render(w, "register.html", PageData{
			Title: "Create Account",
			Flash: &FlashMessage{Type: "error", Message: message},
			Data: map[string]string{
				"tenant_name": input.TenantName,
				"tenant_slug": input.TenantSlug,
				"name":        input.Name,
				"email":       input.Email,
				"timezone":    input.Timezone,
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
