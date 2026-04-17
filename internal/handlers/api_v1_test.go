package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/middleware"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/services"
)

// --- JSON helper tests ---

func TestJsonError(t *testing.T) {
	w := httptest.NewRecorder()
	jsonError(w, "something broke", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "something broke" {
		t.Errorf("expected error message 'something broke', got '%s'", body["error"])
	}
}

func TestJsonOK(t *testing.T) {
	w := httptest.NewRecorder()
	jsonOK(w, map[string]string{"status": "ok"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", body["status"])
	}
}

// --- Model conversion tests ---

func TestToAPIHost(t *testing.T) {
	host := &models.Host{
		ID:             "h1",
		Name:           "Jane Doe",
		Email:          "jane@example.com",
		Slug:           "jane-doe",
		Timezone:       "America/New_York",
		SmartDurations: true,
		IsAdmin:        true,
		PasswordHash:   "secret-hash", // should not appear
	}

	result := toAPIHost(host)
	if result.ID != "h1" {
		t.Errorf("expected ID 'h1', got '%s'", result.ID)
	}
	if result.Name != "Jane Doe" {
		t.Errorf("expected Name 'Jane Doe', got '%s'", result.Name)
	}
	if result.SmartDurations != true {
		t.Error("expected SmartDurations true")
	}

	// Verify sensitive data is not in the JSON output
	data, _ := json.Marshal(result)
	if bytes.Contains(data, []byte("secret-hash")) {
		t.Error("PasswordHash leaked into JSON output")
	}

	// Nil input
	if toAPIHost(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestToAPITenant(t *testing.T) {
	tenant := &models.Tenant{
		ID:   "t1",
		Name: "Acme Corp",
		Slug: "acme",
	}

	result := toAPITenant(tenant)
	if result.ID != "t1" || result.Name != "Acme Corp" || result.Slug != "acme" {
		t.Errorf("unexpected tenant: %+v", result)
	}

	if toAPITenant(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

// --- Login endpoint tests ---

func TestLogin_MissingFields(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	tests := []struct {
		name string
		body string
	}{
		{"empty body", `{}`},
		{"missing password", `{"email":"test@example.com"}`},
		{"missing email", `{"password":"12345678"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.Login(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestLogin_InvalidJSON(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Me endpoint tests ---

func TestMe_Authenticated(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	host := &services.HostWithTenant{
		Host: &models.Host{
			ID:       "h1",
			Name:     "Jane",
			Email:    "jane@example.com",
			Slug:     "jane",
			Timezone: "UTC",
		},
		Tenant: &models.Tenant{
			ID:   "t1",
			Name: "Acme",
			Slug: "acme",
		},
	}

	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	ctx := context.WithValue(req.Context(), middleware.HostKey, host)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.Me(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body map[string]json.RawMessage
	json.NewDecoder(w.Body).Decode(&body)

	var apiH apiHost
	json.Unmarshal(body["host"], &apiH)
	if apiH.ID != "h1" || apiH.Name != "Jane" {
		t.Errorf("unexpected host in response: %+v", apiH)
	}

	var apiT apiTenant
	json.Unmarshal(body["tenant"], &apiT)
	if apiT.ID != "t1" || apiT.Name != "Acme" {
		t.Errorf("unexpected tenant in response: %+v", apiT)
	}
}

func TestMe_Unauthenticated(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	w := httptest.NewRecorder()

	h.Me(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- SelectOrg endpoint tests ---

func TestSelectOrg_MissingFields(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	tests := []struct {
		name string
		body string
	}{
		{"empty body", `{}`},
		{"missing selection_token", `{"host_id":"h1"}`},
		{"missing host_id", `{"selection_token":"tok"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/auth/login/select-org", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.SelectOrg(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

// --- Booking endpoint tests (with mock context) ---

func TestGetBooking_Unauthenticated(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	req := httptest.NewRequest("GET", "/api/v1/bookings/some-id", nil)
	w := httptest.NewRecorder()

	h.GetBooking(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListBookings_Unauthenticated(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	req := httptest.NewRequest("GET", "/api/v1/bookings", nil)
	w := httptest.NewRecorder()

	h.ListBookings(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTodayBookings_Unauthenticated(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	req := httptest.NewRequest("GET", "/api/v1/bookings/today", nil)
	w := httptest.NewRecorder()

	h.TodayBookings(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPendingBookings_Unauthenticated(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	req := httptest.NewRequest("GET", "/api/v1/bookings/pending", nil)
	w := httptest.NewRecorder()

	h.PendingBookings(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestApproveBooking_Unauthenticated(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	req := httptest.NewRequest("POST", "/api/v1/bookings/some-id/approve", nil)
	w := httptest.NewRecorder()

	h.ApproveBooking(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRejectBooking_Unauthenticated(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	req := httptest.NewRequest("POST", "/api/v1/bookings/some-id/reject", nil)
	w := httptest.NewRecorder()

	h.RejectBooking(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCancelBooking_Unauthenticated(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	req := httptest.NewRequest("POST", "/api/v1/bookings/some-id/cancel", nil)
	w := httptest.NewRecorder()

	h.CancelBooking(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- apiBooking serialization test ---

func TestAPIBooking_JSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ab := apiBooking{
		ID:              "b1",
		TemplateName:    "Quick Chat",
		Status:          models.BookingStatusConfirmed,
		StartTime:       now,
		EndTime:         now.Add(30 * time.Minute),
		Duration:        30,
		InviteeName:     "Bob",
		InviteeEmail:    "bob@example.com",
		InviteeTimezone: "Europe/London",
		ConferenceLink:  "https://meet.google.com/abc",
		LocationType:    models.ConferencingProviderGoogleMeet,
		CreatedAt:       now,
	}

	data, err := json.Marshal(ab)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if result["id"] != "b1" {
		t.Errorf("expected id 'b1', got '%v'", result["id"])
	}
	if result["status"] != "confirmed" {
		t.Errorf("expected status 'confirmed', got '%v'", result["status"])
	}
	if result["template_name"] != "Quick Chat" {
		t.Errorf("expected template_name 'Quick Chat', got '%v'", result["template_name"])
	}
	if result["location_type"] != "google_meet" {
		t.Errorf("expected location_type 'google_meet', got '%v'", result["location_type"])
	}
	if result["conference_link"] != "https://meet.google.com/abc" {
		t.Errorf("expected conference_link, got '%v'", result["conference_link"])
	}
}

// --- Logout endpoint test ---

func TestLogout_NoToken(t *testing.T) {
	h := &APIV1Handler{handlers: &Handlers{}}

	req := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()

	h.Logout(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// --- Native OAuth helper tests ---

func TestIsNativeFlow(t *testing.T) {
	tests := []struct {
		state    string
		expected bool
	}{
		{"auth:native:abc123", true},
		{"auth:login:abc123", false},
		{"auth:signup:abc123", false},
		{"", false},
		{"native", false},
	}

	for _, tt := range tests {
		if got := isNativeFlow(tt.state); got != tt.expected {
			t.Errorf("isNativeFlow(%q) = %v, want %v", tt.state, got, tt.expected)
		}
	}
}

func TestNativeFlowErrorRedirect(t *testing.T) {
	req := httptest.NewRequest("GET", "/auth/google/auth-callback", nil)
	w := httptest.NewRecorder()

	nativeFlowErrorRedirect(w, req, "something failed", "https://meet.example.com")

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if loc == "" {
		t.Fatal("expected Location header")
	}
	if !bytes.Contains([]byte(loc), []byte("meetwhenbar://")) {
		t.Errorf("expected meetwhenbar:// URL scheme, got %s", loc)
	}
	if !bytes.Contains([]byte(loc), []byte("error=")) {
		t.Errorf("expected error parameter in redirect, got %s", loc)
	}
	if !bytes.Contains([]byte(loc), []byte("server=")) {
		t.Errorf("expected server parameter in redirect, got %s", loc)
	}
}

func TestNativeRedirect_withToken(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.BaseURL = "https://meet.example.com"
	h := &APIV1Handler{handlers: &Handlers{cfg: cfg}}

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	h.nativeRedirect(w, req, "test-token-123", "")

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if !bytes.Contains([]byte(loc), []byte("token=test-token-123")) {
		t.Errorf("expected token in redirect URL, got %s", loc)
	}
	if bytes.Contains([]byte(loc), []byte("error=")) {
		t.Errorf("should not have error param when token is set, got %s", loc)
	}
}

func TestNativeRedirect_withError(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.BaseURL = "https://meet.example.com"
	h := &APIV1Handler{handlers: &Handlers{cfg: cfg}}

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	h.nativeRedirect(w, req, "", "account not found")

	loc := w.Header().Get("Location")
	if bytes.Contains([]byte(loc), []byte("token=")) {
		t.Errorf("should not have token param on error, got %s", loc)
	}
	if !bytes.Contains([]byte(loc), []byte("error=")) {
		t.Errorf("expected error param, got %s", loc)
	}
}
