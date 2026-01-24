package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

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

// CreateMeeting creates a video conference meeting
func (s *ConferencingService) CreateMeeting(ctx context.Context, details *BookingWithDetails) (string, error) {
	switch details.Template.LocationType {
	case models.ConferencingProviderGoogleMeet:
		// Google Meet is handled by calendar event creation
		return "", nil
	case models.ConferencingProviderZoom:
		return s.createZoomMeeting(ctx, details)
	case models.ConferencingProviderPhone:
		return details.Template.CustomLocation, nil
	case models.ConferencingProviderCustom:
		return details.Template.CustomLocation, nil
	}
	return "", nil
}

// ConnectZoom connects Zoom account via OAuth
func (s *ConferencingService) ConnectZoom(ctx context.Context, hostID, authCode, redirectURI string) (*models.ConferencingConnection, error) {
	tokens, err := s.exchangeZoomAuthCode(authCode, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange auth code: %w", err)
	}

	now := time.Now()
	expiry := now.Add(time.Duration(tokens.ExpiresIn) * time.Second)

	// Check if connection already exists
	existing, _ := s.repos.Conferencing.GetByHostAndProvider(ctx, hostID, models.ConferencingProviderZoom)
	if existing != nil {
		existing.AccessToken = tokens.AccessToken
		existing.RefreshToken = tokens.RefreshToken
		existing.TokenExpiry = &expiry
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
	defer resp.Body.Close()

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

func (s *ConferencingService) refreshZoomToken(conn *models.ConferencingConnection) error {
	if conn.TokenExpiry == nil || time.Now().Before(conn.TokenExpiry.Add(-5*time.Minute)) {
		return nil
	}

	data := fmt.Sprintf("refresh_token=%s&grant_type=refresh_token", conn.RefreshToken)

	req, _ := http.NewRequest("POST", "https://zoom.us/oauth/token", strings.NewReader(data))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.cfg.OAuth.Zoom.ClientID, s.cfg.OAuth.Zoom.ClientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to refresh token")
	}

	var tokens zoomTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return err
	}

	conn.AccessToken = tokens.AccessToken
	if tokens.RefreshToken != "" {
		conn.RefreshToken = tokens.RefreshToken
	}
	expiry := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	conn.TokenExpiry = &expiry

	return s.repos.Conferencing.Update(context.Background(), conn)
}

func (s *ConferencingService) createZoomMeeting(ctx context.Context, details *BookingWithDetails) (string, error) {
	conn, err := s.repos.Conferencing.GetByHostAndProvider(ctx, details.Booking.HostID, models.ConferencingProviderZoom)
	if err != nil || conn == nil {
		return "", fmt.Errorf("zoom not connected")
	}

	if err := s.refreshZoomToken(conn); err != nil {
		return "", err
	}

	meeting := map[string]interface{}{
		"topic":      details.Template.Name + " with " + details.Booking.InviteeName,
		"type":       2, // Scheduled meeting
		"start_time": details.Booking.StartTime.Format("2006-01-02T15:04:05Z"),
		"duration":   details.Booking.Duration,
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
	defer resp.Body.Close()

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
