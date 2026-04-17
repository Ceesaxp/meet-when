package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/meet-when/meet-when/internal/middleware"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/services"
)

// APIV1Handler handles JSON API v1 endpoints for native clients (menu-bar app, etc.).
type APIV1Handler struct {
	handlers *Handlers
}

// --- JSON helpers ---

func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// --- Auth endpoints ---

// loginRequest is the JSON body for POST /api/v1/auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginResponse is the JSON response for a successful login.
type loginResponse struct {
	Token                string       `json:"token,omitempty"`
	RequiresOrgSelection bool         `json:"requires_org_selection"`
	Orgs                 []orgOption  `json:"orgs,omitempty"`
	Host                 *apiHost     `json:"host,omitempty"`
	Tenant               *apiTenant   `json:"tenant,omitempty"`
}

type orgOption struct {
	TenantID       string `json:"tenant_id"`
	TenantSlug     string `json:"tenant_slug"`
	TenantName     string `json:"tenant_name"`
	HostID         string `json:"host_id"`
	SelectionToken string `json:"selection_token"`
}

type selectOrgRequest struct {
	HostID         string `json:"host_id"`
	SelectionToken string `json:"selection_token"`
}

// apiHost is the public JSON representation of a host (no sensitive fields).
type apiHost struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Slug           string `json:"slug"`
	Timezone       string `json:"timezone"`
	SmartDurations bool   `json:"smart_durations"`
	IsAdmin        bool   `json:"is_admin"`
}

// apiTenant is the public JSON representation of a tenant.
type apiTenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func toAPIHost(h *models.Host) *apiHost {
	if h == nil {
		return nil
	}
	return &apiHost{
		ID:             h.ID,
		Name:           h.Name,
		Email:          h.Email,
		Slug:           h.Slug,
		Timezone:       h.Timezone,
		SmartDurations: h.SmartDurations,
		IsAdmin:        h.IsAdmin,
	}
}

func toAPITenant(t *models.Tenant) *apiTenant {
	if t == nil {
		return nil
	}
	return &apiTenant{
		ID:   t.ID,
		Name: t.Name,
		Slug: t.Slug,
	}
}

// Login handles POST /api/v1/auth/login.
func (h *APIV1Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" {
		jsonError(w, "email and password are required", http.StatusBadRequest)
		return
	}

	result, err := h.handlers.services.Auth.SimplifiedLogin(r.Context(), services.SimplifiedLoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		if err == services.ErrInvalidCredentials {
			jsonError(w, "invalid email or password", http.StatusUnauthorized)
			return
		}
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	resp := loginResponse{
		RequiresOrgSelection: result.RequiresOrgSelection,
	}

	if result.RequiresOrgSelection {
		for _, org := range result.AvailableOrgs {
			resp.Orgs = append(resp.Orgs, orgOption{
				TenantID:       org.TenantID,
				TenantSlug:     org.TenantSlug,
				TenantName:     org.TenantName,
				HostID:         org.HostID,
				SelectionToken: org.SelectionToken,
			})
		}
	} else {
		resp.Token = result.SessionToken
		resp.Host = toAPIHost(result.Host)
		resp.Tenant = toAPITenant(result.Tenant)
	}

	jsonOK(w, resp)
}

// SelectOrg handles POST /api/v1/auth/login/select-org.
func (h *APIV1Handler) SelectOrg(w http.ResponseWriter, r *http.Request) {
	var req selectOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.HostID == "" || req.SelectionToken == "" {
		jsonError(w, "host_id and selection_token are required", http.StatusBadRequest)
		return
	}

	hostWithTenant, sessionToken, err := h.handlers.services.Auth.CompleteOrgSelection(r.Context(), services.CompleteOrgSelectionInput{
		HostID:         req.HostID,
		SelectionToken: req.SelectionToken,
	})
	if err != nil {
		if err == services.ErrInvalidSelectionToken {
			jsonError(w, "invalid or expired selection token", http.StatusUnauthorized)
			return
		}
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, loginResponse{
		Token:  sessionToken,
		Host:   toAPIHost(hostWithTenant.Host),
		Tenant: toAPITenant(hostWithTenant.Tenant),
	})
}

// Logout handles POST /api/v1/auth/logout (requires auth).
func (h *APIV1Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token, _ := middleware.ExtractSessionToken(r)
	if token == "" {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.handlers.services.Session.DeleteSession(r.Context(), token); err != nil {
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

// Me handles GET /api/v1/me (requires auth).
func (h *APIV1Handler) Me(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	jsonOK(w, map[string]interface{}{
		"host":   toAPIHost(host.Host),
		"tenant": toAPITenant(host.Tenant),
	})
}

// --- Booking endpoints ---

// apiBooking is the public JSON representation of a booking.
type apiBooking struct {
	ID              string                      `json:"id"`
	TemplateName    string                      `json:"template_name"`
	Status          models.BookingStatus        `json:"status"`
	StartTime       time.Time                   `json:"start_time"`
	EndTime         time.Time                   `json:"end_time"`
	Duration        int                         `json:"duration"`
	InviteeName     string                      `json:"invitee_name"`
	InviteeEmail    string                      `json:"invitee_email"`
	InviteeTimezone string                      `json:"invitee_timezone"`
	ConferenceLink  string                      `json:"conference_link"`
	LocationType    models.ConferencingProvider `json:"location_type"`
	CreatedAt       time.Time                   `json:"created_at"`
}

func (h *APIV1Handler) toAPIBooking(ctx context.Context, b *models.Booking) apiBooking {
	ab := apiBooking{
		ID:              b.ID,
		Status:          b.Status,
		StartTime:       b.StartTime.Time,
		EndTime:         b.EndTime.Time,
		Duration:        b.Duration,
		InviteeName:     b.InviteeName,
		InviteeEmail:    b.InviteeEmail,
		InviteeTimezone: b.InviteeTimezone,
		ConferenceLink:  b.ConferenceLink,
		CreatedAt:       b.CreatedAt.Time,
	}

	// Populate template name and location type
	template, _ := h.handlers.repos.Template.GetByID(ctx, b.TemplateID)
	if template != nil {
		ab.TemplateName = template.Name
		ab.LocationType = template.LocationType
	}

	return ab
}

// ListBookings handles GET /api/v1/bookings.
func (h *APIV1Handler) ListBookings(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var statusFilter *models.BookingStatus
	if s := r.URL.Query().Get("status"); s != "" {
		bs := models.BookingStatus(s)
		statusFilter = &bs
	}
	includeArchived := r.URL.Query().Get("include_archived") == "true"

	bookings, err := h.handlers.services.Booking.GetBookings(r.Context(), host.Host.ID, statusFilter, includeArchived)
	if err != nil {
		jsonError(w, "failed to fetch bookings", http.StatusInternalServerError)
		return
	}

	result := make([]apiBooking, 0, len(bookings))
	for _, b := range bookings {
		result = append(result, h.toAPIBooking(r.Context(), b))
	}

	jsonOK(w, map[string]interface{}{"bookings": result})
}

// TodayBookings handles GET /api/v1/bookings/today.
func (h *APIV1Handler) TodayBookings(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get confirmed bookings
	status := models.BookingStatusConfirmed
	bookings, err := h.handlers.services.Booking.GetBookings(r.Context(), host.Host.ID, &status, false)
	if err != nil {
		jsonError(w, "failed to fetch bookings", http.StatusInternalServerError)
		return
	}

	// Filter to today in the host's timezone
	loc, err := time.LoadLocation(host.Host.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24 * time.Hour)

	var today []apiBooking
	for _, b := range bookings {
		bStart := b.StartTime.Time.In(loc)
		if !bStart.Before(startOfDay) && bStart.Before(endOfDay) {
			today = append(today, h.toAPIBooking(r.Context(), b))
		}
	}

	if today == nil {
		today = []apiBooking{}
	}

	jsonOK(w, map[string]interface{}{"bookings": today})
}

// PendingBookings handles GET /api/v1/bookings/pending.
func (h *APIV1Handler) PendingBookings(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	bookings, err := h.handlers.services.Booking.GetPendingBookings(r.Context(), host.Host.ID)
	if err != nil {
		jsonError(w, "failed to fetch bookings", http.StatusInternalServerError)
		return
	}

	result := make([]apiBooking, 0, len(bookings))
	for _, b := range bookings {
		result = append(result, h.toAPIBooking(r.Context(), b))
	}

	jsonOK(w, map[string]interface{}{"bookings": result})
}

// GetBooking handles GET /api/v1/bookings/{id}.
func (h *APIV1Handler) GetBooking(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	bookingID := r.PathValue("id")
	booking, err := h.handlers.services.Booking.GetBooking(r.Context(), bookingID)
	if err != nil || booking == nil || booking.HostID != host.Host.ID {
		jsonError(w, "booking not found", http.StatusNotFound)
		return
	}

	jsonOK(w, map[string]interface{}{"booking": h.toAPIBooking(r.Context(), booking)})
}

// ApproveBooking handles POST /api/v1/bookings/{id}/approve.
func (h *APIV1Handler) ApproveBooking(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	bookingID := r.PathValue("id")
	details, err := h.handlers.services.Booking.ApproveBooking(r.Context(), host.Host.ID, host.Tenant.ID, bookingID)
	if err != nil {
		if err == services.ErrBookingNotFound {
			jsonError(w, "booking not found", http.StatusNotFound)
			return
		}
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	jsonOK(w, map[string]interface{}{"booking": h.toAPIBooking(r.Context(), details.Booking)})
}

// rejectRequest is the JSON body for POST /api/v1/bookings/{id}/reject.
type rejectRequest struct {
	Reason string `json:"reason"`
}

// RejectBooking handles POST /api/v1/bookings/{id}/reject.
func (h *APIV1Handler) RejectBooking(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req rejectRequest
	json.NewDecoder(r.Body).Decode(&req) // reason is optional

	bookingID := r.PathValue("id")
	err := h.handlers.services.Booking.RejectBooking(r.Context(), host.Host.ID, host.Tenant.ID, bookingID, req.Reason)
	if err != nil {
		if err == services.ErrBookingNotFound {
			jsonError(w, "booking not found", http.StatusNotFound)
			return
		}
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	jsonOK(w, map[string]string{"status": "rejected"})
}

// cancelRequest is the JSON body for POST /api/v1/bookings/{id}/cancel.
type cancelRequest struct {
	Reason string `json:"reason"`
}

// CancelBooking handles POST /api/v1/bookings/{id}/cancel.
func (h *APIV1Handler) CancelBooking(w http.ResponseWriter, r *http.Request) {
	host := middleware.GetHost(r.Context())
	if host == nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req cancelRequest
	json.NewDecoder(r.Body).Decode(&req)

	bookingID := r.PathValue("id")

	// Verify the booking belongs to this host
	booking, err := h.handlers.services.Booking.GetBooking(r.Context(), bookingID)
	if err != nil || booking == nil || booking.HostID != host.Host.ID {
		jsonError(w, "booking not found", http.StatusNotFound)
		return
	}

	err = h.handlers.services.Booking.CancelBooking(r.Context(), bookingID, "host", req.Reason)
	if err != nil {
		if err == services.ErrBookingNotFound {
			jsonError(w, "booking not found", http.StatusNotFound)
			return
		}
		if err == services.ErrBookingCancelled {
			jsonError(w, "booking already cancelled", http.StatusBadRequest)
			return
		}
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	jsonOK(w, map[string]string{"status": "cancelled"})
}

// --- Google OAuth for native clients ---

// GoogleLogin handles GET /api/v1/auth/google.
// Starts the Google OAuth flow for native app clients.
// The browser is opened by the native app; after Google auth completes,
// the server redirects to meetwhenbar://auth/callback?token=xxx.
func (h *APIV1Handler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	authURL, nonce, err := h.handlers.services.Auth.GetGoogleAuthURL("native")
	if err != nil {
		log.Printf("Error generating Google auth URL for native: %v", err)
		jsonError(w, "failed to start Google authentication", http.StatusInternalServerError)
		return
	}

	// Store nonce in HttpOnly cookie for CSRF verification on callback
	http.SetCookie(w, &http.Cookie{
		Name:     "google_auth_nonce",
		Value:    nonce,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, authURL, http.StatusFound)
}

// GoogleCallback handles the OAuth callback when the flow is "native".
// Instead of rendering HTML, it redirects to the meetwhenbar:// URL scheme
// with the session token as a query parameter.
func (h *APIV1Handler) GoogleCallback(w http.ResponseWriter, r *http.Request, userInfo *services.GoogleUserInfo) {
	result, err := h.handlers.services.Auth.LoginWithGoogle(r.Context(), userInfo.Sub, userInfo.Email)
	if err != nil {
		if err == services.ErrGoogleAccountNotFound {
			h.nativeRedirect(w, r, "", "No account found for this Google identity. Please sign up first.")
			return
		}
		log.Printf("Native Google login error: %v", err)
		h.nativeRedirect(w, r, "", "Login failed. Please try again.")
		return
	}

	// Multi-org case: for native flow, we pick the first org or redirect with selection info.
	// Since URL schemes have limited space, we redirect with a selection_required flag
	// and the native app falls back to email/password login for org selection.
	if result.RequiresOrgSelection {
		// Create a session for the first org automatically for simplicity,
		// or signal the native app to handle org selection.
		if len(result.AvailableOrgs) == 1 {
			// Single org option — auto-select
			hostWithTenant, sessionToken, err := h.handlers.services.Auth.CompleteOrgSelection(r.Context(), services.CompleteOrgSelectionInput{
				HostID:         result.AvailableOrgs[0].HostID,
				SelectionToken: result.AvailableOrgs[0].SelectionToken,
			})
			if err == nil {
				h.nativeRedirect(w, r, sessionToken, "")

				// Also pass host/tenant info as fragments (won't be sent to server)
				_ = hostWithTenant
				return
			}
		}

		// Multiple orgs — signal the app to use email/password flow for org selection
		h.nativeRedirect(w, r, "", "Multiple organizations found. Please use email/password to select your organization.")
		return
	}

	h.nativeRedirect(w, r, result.SessionToken, "")
}

// nativeRedirect sends a redirect to the meetwhenbar:// URL scheme.
func (h *APIV1Handler) nativeRedirect(w http.ResponseWriter, r *http.Request, token, errMsg string) {
	u := url.URL{
		Scheme: "meetwhenbar",
		Host:   "auth",
		Path:   "/callback",
	}
	q := u.Query()
	if token != "" {
		q.Set("token", token)
	}
	if errMsg != "" {
		q.Set("error", errMsg)
	}
	// Pass the server's base URL so the app knows which server issued the token
	q.Set("server", strings.TrimRight(h.handlers.cfg.Server.BaseURL, "/"))
	u.RawQuery = q.Encode()

	http.Redirect(w, r, u.String(), http.StatusFound)
}

// isNativeFlow checks if the OAuth state indicates a native app flow.
func isNativeFlow(state string) bool {
	parts := strings.Split(state, ":")
	return len(parts) >= 2 && parts[1] == "native"
}

// nativeFlowErrorRedirect sends an error back to the native app via URL scheme
// when the callback fails before we can determine the flow type.
func nativeFlowErrorRedirect(w http.ResponseWriter, r *http.Request, msg string, baseURL string) {
	u := fmt.Sprintf("meetwhenbar://auth/callback?error=%s&server=%s",
		url.QueryEscape(msg),
		url.QueryEscape(strings.TrimRight(baseURL, "/")))
	http.Redirect(w, r, u, http.StatusFound)
}
