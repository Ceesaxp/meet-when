package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// ErrConferencingReauthRequired indicates that the host's conferencing OAuth
// token cannot be refreshed (revoked, expired beyond grace, or invalid) and the
// host must reconnect via the OAuth flow. Callers in the booking path treat
// this differently from transient errors: the booking still confirms but the
// host is notified and the connection is flagged for reconnect.
var ErrConferencingReauthRequired = errors.New("conferencing reauth required")

// ConferencingService handles video conferencing operations
type ConferencingService struct {
	cfg   *config.Config
	repos *repository.Repositories
}

// NewConferencingService creates a new conferencing service
func NewConferencingService(cfg *config.Config, repos *repository.Repositories) *ConferencingService {
	return &ConferencingService{
		cfg:   cfg,
		repos: repos,
	}
}

// CreateMeeting creates a video conference meeting for a booking. Kept on
// the original signature; delegates to the booking-shape-agnostic raw helper
// for the Zoom path so HostedEventService can reuse the same plumbing.
func (s *ConferencingService) CreateMeeting(ctx context.Context, details *BookingWithDetails) (string, error) {
	switch details.Template.LocationType {
	case models.ConferencingProviderGoogleMeet:
		// Google Meet is handled by calendar event creation
		return "", nil
	case models.ConferencingProviderZoom:
		topic := details.Template.Name + " with " + details.Booking.InviteeName
		return s.createZoomMeetingRaw(ctx, details.Booking.HostID, topic, details.Booking.StartTime.Time, details.Booking.Duration)
	case models.ConferencingProviderPhone:
		return details.Template.CustomLocation, nil
	case models.ConferencingProviderCustom:
		return details.Template.CustomLocation, nil
	}
	return "", nil
}

// CreateMeetingForHostedEvent is the host-driven equivalent of CreateMeeting.
// Returns the conference link (or "") for the requested location type.
func (s *ConferencingService) CreateMeetingForHostedEvent(ctx context.Context, hostID, title string, start time.Time, durationMin int, locationType models.ConferencingProvider, customLocation string) (string, error) {
	switch locationType {
	case models.ConferencingProviderGoogleMeet:
		return "", nil
	case models.ConferencingProviderZoom:
		return s.createZoomMeetingRaw(ctx, hostID, title, start, durationMin)
	case models.ConferencingProviderPhone, models.ConferencingProviderCustom:
		return customLocation, nil
	}
	return "", nil
}

// ConnectZoom connects Zoom account via OAuth
func (s *ConferencingService) ConnectZoom(ctx context.Context, hostID, authCode, redirectURI string) (*models.ConferencingConnection, error) {
	tokens, err := s.exchangeZoomAuthCode(authCode, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange auth code: %w", err)
	}

	now := models.Now()
	expiry := models.NewSQLiteTime(time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second))

	// Check if connection already exists
	existing, _ := s.repos.Conferencing.GetByHostAndProvider(ctx, hostID, models.ConferencingProviderZoom)
	if existing != nil {
		existing.AccessToken = tokens.AccessToken
		existing.RefreshToken = tokens.RefreshToken
		existing.TokenExpiry = &expiry
		// A successful reconnect clears any prior reauth state.
		existing.LastRefreshError = sql.NullString{}
		if err := s.repos.Conferencing.Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	conn := &models.ConferencingConnection{
		ID:           uuid.New().String(),
		HostID:       hostID,
		Provider:     models.ConferencingProviderZoom,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenExpiry:  &expiry,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repos.Conferencing.Create(ctx, conn); err != nil {
		return nil, err
	}

	return conn, nil
}

// GetZoomAuthURL returns the Zoom OAuth URL
func (s *ConferencingService) GetZoomAuthURL(state string) string {
	return fmt.Sprintf(
		"https://zoom.us/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=%s",
		s.cfg.OAuth.Zoom.ClientID,
		s.cfg.OAuth.Zoom.RedirectURL,
		state,
	)
}

// GetConnections returns all conferencing connections for a host
func (s *ConferencingService) GetConnections(ctx context.Context, hostID string) ([]*models.ConferencingConnection, error) {
	return s.repos.Conferencing.GetByHostID(ctx, hostID)
}

// DisconnectProvider disconnects a conferencing provider
func (s *ConferencingService) DisconnectProvider(ctx context.Context, hostID string, provider models.ConferencingProvider) error {
	conn, err := s.repos.Conferencing.GetByHostAndProvider(ctx, hostID, provider)
	if err != nil || conn == nil {
		return nil
	}
	return s.repos.Conferencing.Delete(ctx, conn.ID)
}

// Zoom API implementations

type zoomTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func (s *ConferencingService) exchangeZoomAuthCode(code, redirectURI string) (*zoomTokenResponse, error) {
	data := fmt.Sprintf(
		"code=%s&grant_type=authorization_code&redirect_uri=%s",
		code, redirectURI,
	)

	req, _ := http.NewRequest("POST", "https://zoom.us/oauth/token", strings.NewReader(data))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.cfg.OAuth.Zoom.ClientID, s.cfg.OAuth.Zoom.ClientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var tokens zoomTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, err
	}

	return &tokens, nil
}

// zoomTokenError is the OAuth-style error body Zoom returns on token endpoint
// failures (e.g. {"reason":"Invalid Token!","error":"invalid_grant"}).
type zoomTokenError struct {
	Error  string `json:"error"`
	Reason string `json:"reason"`
}

// isPermanentTokenError reports whether the Zoom OAuth error code indicates a
// non-recoverable refresh-token failure (revoked / expired beyond grace /
// invalid). Transient HTTP/network errors and 5xx responses are NOT permanent.
func isPermanentTokenError(code string) bool {
	switch code {
	case "invalid_grant", "invalid_request", "invalid_client", "unauthorized_client", "unsupported_grant_type":
		return true
	}
	return false
}

func (s *ConferencingService) refreshZoomToken(ctx context.Context, conn *models.ConferencingConnection) error {
	// Skip refresh if we already have a valid, unexpired token with >5 min remaining.
	if conn.TokenExpiry != nil && time.Now().Before(conn.TokenExpiry.Add(-5*time.Minute)) {
		return nil
	}

	// If we previously flagged this connection as needing reauth and the token
	// is also expired, fail fast — refresh would only fail again. The host must
	// reconnect via OAuth.
	if conn.NeedsReauth() {
		return ErrConferencingReauthRequired
	}

	data := fmt.Sprintf("refresh_token=%s&grant_type=refresh_token", conn.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://zoom.us/oauth/token", strings.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.cfg.OAuth.Zoom.ClientID, s.cfg.OAuth.Zoom.ClientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Network/transport error — transient, do not flag connection.
		return fmt.Errorf("zoom token refresh transport error: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var oauthErr zoomTokenError
		_ = json.Unmarshal(body, &oauthErr)
		// 4xx with a known permanent error code → require reauth.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && isPermanentTokenError(oauthErr.Error) {
			conn.LastRefreshError = sql.NullString{
				String: fmt.Sprintf("%s: %s", oauthErr.Error, oauthErr.Reason),
				Valid:  true,
			}
			if upErr := s.repos.Conferencing.Update(ctx, conn); upErr != nil {
				log.Printf("[ZOOM] failed to persist reauth flag for conn=%s: %v", conn.ID, upErr)
			}
			log.Printf("[ZOOM] reauth required for host=%s: %s (%s)", conn.HostID, oauthErr.Error, oauthErr.Reason)
			return ErrConferencingReauthRequired
		}
		// Other 4xx (rate limit-ish) and 5xx → transient.
		return fmt.Errorf("zoom token refresh failed (status=%d): %s", resp.StatusCode, string(body))
	}

	var tokens zoomTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return err
	}

	conn.AccessToken = tokens.AccessToken
	if tokens.RefreshToken != "" {
		conn.RefreshToken = tokens.RefreshToken
	}
	expiry := models.NewSQLiteTime(time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second))
	conn.TokenExpiry = &expiry
	conn.LastRefreshError = sql.NullString{} // clear on success

	return s.repos.Conferencing.Update(ctx, conn)
}

// createZoomMeetingRaw is the shape-agnostic Zoom meeting creator. Used by
// both booking and hosted-event flows. Returns the meeting join URL.
func (s *ConferencingService) createZoomMeetingRaw(ctx context.Context, hostID, topic string, start time.Time, durationMin int) (string, error) {
	conn, err := s.repos.Conferencing.GetByHostAndProvider(ctx, hostID, models.ConferencingProviderZoom)
	if err != nil || conn == nil {
		return "", ErrConferencingReauthRequired
	}

	if err := s.refreshZoomToken(ctx, conn); err != nil {
		return "", err
	}

	meeting := map[string]interface{}{
		"topic":      topic,
		"type":       2, // Scheduled meeting
		"start_time": start.UTC().Format("2006-01-02T15:04:05Z"),
		"duration":   durationMin,
		"timezone":   "UTC",
		"settings": map[string]interface{}{
			"host_video":        true,
			"participant_video": true,
			"join_before_host":  true,
			"mute_upon_entry":   false,
		},
	}

	body, _ := json.Marshal(meeting)
	req, _ := http.NewRequest("POST", "https://api.zoom.us/v2/users/me/meetings", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+conn.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create meeting: %s", string(respBody))
	}

	var result struct {
		JoinURL string `json:"join_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.JoinURL, nil
}
