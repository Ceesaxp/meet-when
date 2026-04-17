package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/services"
)

// mockSessionService is a minimal SessionService for testing.
// We construct a real SessionService but override behavior via the DB.
// For unit tests, we use ExtractSessionToken directly and test the middleware
// integration through a fake handler.

func TestExtractSessionToken_BearerHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer test-token-123")

	token, isBearer := ExtractSessionToken(req)
	if token != "test-token-123" {
		t.Errorf("expected token 'test-token-123', got '%s'", token)
	}
	if !isBearer {
		t.Error("expected isBearer to be true")
	}
}

func TestExtractSessionToken_Cookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "cookie-token-456"})

	token, isBearer := ExtractSessionToken(req)
	if token != "cookie-token-456" {
		t.Errorf("expected token 'cookie-token-456', got '%s'", token)
	}
	if isBearer {
		t.Error("expected isBearer to be false")
	}
}

func TestExtractSessionToken_BearerTakesPrecedence(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer bearer-token")
	req.AddCookie(&http.Cookie{Name: "session", Value: "cookie-token"})

	token, isBearer := ExtractSessionToken(req)
	if token != "bearer-token" {
		t.Errorf("expected Bearer token to take precedence, got '%s'", token)
	}
	if !isBearer {
		t.Error("expected isBearer to be true")
	}
}

func TestExtractSessionToken_NoAuth(t *testing.T) {
	req := httptest.NewRequest("GET", "/dashboard", nil)

	token, isBearer := ExtractSessionToken(req)
	if token != "" {
		t.Errorf("expected empty token, got '%s'", token)
	}
	if isBearer {
		t.Error("expected isBearer to be false")
	}
}

func TestIsAPIRequest(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/api/v1/me", true},
		{"/api/v1/bookings", true},
		{"/api/timezones", true},
		{"/dashboard", false},
		{"/auth/login", false},
		{"/m/acme/john/intro", false},
	}
	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.path, nil)
		if got := isAPIRequest(req); got != tt.expected {
			t.Errorf("isAPIRequest(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestRequireAuth_NoToken_APIRequest(t *testing.T) {
	// Create a minimal SessionService - won't be called since no token
	handler := RequireAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestRequireAuth_NoToken_BrowserRequest(t *testing.T) {
	handler := RequireAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/dashboard", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/auth/login" {
		t.Errorf("expected redirect to /auth/login, got %s", loc)
	}
}

func TestGetHost(t *testing.T) {
	host := &services.HostWithTenant{
		Host:   &models.Host{ID: "host-1", Name: "Test"},
		Tenant: &models.Tenant{ID: "tenant-1", Name: "Acme"},
	}

	ctx := context.WithValue(context.Background(), HostKey, host)
	got := GetHost(ctx)
	if got == nil || got.Host.ID != "host-1" {
		t.Error("expected to retrieve host from context")
	}

	// Empty context
	got = GetHost(context.Background())
	if got != nil {
		t.Error("expected nil from empty context")
	}
}
