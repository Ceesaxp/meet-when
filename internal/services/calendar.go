package services

import (
	"context"
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

var (
	ErrCalendarNotFound = errors.New("calendar not found")
	ErrCalendarAuth     = errors.New("calendar authentication failed")
)

// CalendarService handles calendar operations
type CalendarService struct {
	cfg   *config.Config
	repos *repository.Repositories
}

// NewCalendarService creates a new calendar service
func NewCalendarService(cfg *config.Config, repos *repository.Repositories) *CalendarService {
	return &CalendarService{
		cfg:   cfg,
		repos: repos,
	}
}

// GoogleCalendarConnectInput represents input for connecting Google Calendar
type GoogleCalendarConnectInput struct {
	HostID       string
	AuthCode     string
	RedirectURI  string
}

// ConnectGoogleCalendar connects a Google Calendar using OAuth
func (s *CalendarService) ConnectGoogleCalendar(ctx context.Context, input GoogleCalendarConnectInput) (*models.CalendarConnection, error) {
	// Exchange auth code for tokens
	tokens, err := s.exchangeGoogleAuthCode(input.AuthCode, input.RedirectURI)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange auth code: %w", err)
	}

	// Get calendar list to find primary calendar
	calendarInfo, err := s.getGoogleCalendarInfo(tokens.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get calendar info: %w", err)
	}

	now := models.Now()
	expiry := models.NewSQLiteTime(time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second))

	calendar := &models.CalendarConnection{
		ID:           uuid.New().String(),
		HostID:       input.HostID,
		Provider:     models.CalendarProviderGoogle,
		Name:         calendarInfo.Name,
		CalendarID:   calendarInfo.ID,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenExpiry:  &expiry,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repos.Calendar.Create(ctx, calendar); err != nil {
		return nil, err
	}

	// Set as default if first calendar
	calendars, _ := s.repos.Calendar.GetByHostID(ctx, input.HostID)
	if len(calendars) == 1 {
		_ = s.repos.Calendar.SetDefault(ctx, input.HostID, calendar.ID)
		calendar.IsDefault = true
	}

	return calendar, nil
}

// CalDAVConnectInput represents input for connecting a CalDAV calendar
type CalDAVConnectInput struct {
	HostID   string
	Name     string
	URL      string
	Username string
	Password string
}

// ConnectCalDAV connects a CalDAV calendar
func (s *CalendarService) ConnectCalDAV(ctx context.Context, input CalDAVConnectInput) (*models.CalendarConnection, error) {
	// Validate CalDAV connection
	if err := s.validateCalDAVConnection(input.URL, input.Username, input.Password); err != nil {
		return nil, fmt.Errorf("failed to validate CalDAV connection: %w", err)
	}

	now := models.Now()
	calendar := &models.CalendarConnection{
		ID:             uuid.New().String(),
		HostID:         input.HostID,
		Provider:       models.CalendarProviderCalDAV,
		Name:           input.Name,
		CalDAVURL:      input.URL,
		CalDAVUsername: input.Username,
		CalDAVPassword: input.Password, // Should be encrypted in production
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repos.Calendar.Create(ctx, calendar); err != nil {
		return nil, err
	}

	// Set as default if first calendar
	calendars, _ := s.repos.Calendar.GetByHostID(ctx, input.HostID)
	if len(calendars) == 1 {
		_ = s.repos.Calendar.SetDefault(ctx, input.HostID, calendar.ID)
		calendar.IsDefault = true
	}

	return calendar, nil
}

// DisconnectCalendar removes a calendar connection
func (s *CalendarService) DisconnectCalendar(ctx context.Context, hostID, calendarID string) error {
	cal, err := s.repos.Calendar.GetByID(ctx, calendarID)
	if err != nil || cal == nil || cal.HostID != hostID {
		return ErrCalendarNotFound
	}

	return s.repos.Calendar.Delete(ctx, calendarID)
}

// SetDefaultCalendar sets a calendar as the default
func (s *CalendarService) SetDefaultCalendar(ctx context.Context, hostID, calendarID string) error {
	cal, err := s.repos.Calendar.GetByID(ctx, calendarID)
	if err != nil || cal == nil || cal.HostID != hostID {
		return ErrCalendarNotFound
	}

	return s.repos.Calendar.SetDefault(ctx, hostID, calendarID)
}

// GetCalendars returns all calendars for a host
func (s *CalendarService) GetCalendars(ctx context.Context, hostID string) ([]*models.CalendarConnection, error) {
	return s.repos.Calendar.GetByHostID(ctx, hostID)
}

// GetBusyTimes returns busy times from all connected calendars
func (s *CalendarService) GetBusyTimes(ctx context.Context, hostID string, start, end time.Time) ([]models.TimeSlot, error) {
	calendars, err := s.repos.Calendar.GetByHostID(ctx, hostID)
	if err != nil {
		return nil, err
	}

	var allBusyTimes []models.TimeSlot

	for _, cal := range calendars {
		var busyTimes []models.TimeSlot
		var fetchErr error

		switch cal.Provider {
		case models.CalendarProviderGoogle:
			busyTimes, fetchErr = s.getGoogleBusyTimes(ctx, cal, start, end)
		case models.CalendarProviderCalDAV, models.CalendarProviderICloud:
			busyTimes, fetchErr = s.getCalDAVBusyTimes(ctx, cal, start, end)
		}

		if fetchErr != nil {
			// Log but continue with other calendars
			continue
		}

		allBusyTimes = append(allBusyTimes, busyTimes...)
	}

	return allBusyTimes, nil
}

// CreateEvent creates a calendar event for a booking
func (s *CalendarService) CreateEvent(ctx context.Context, details *BookingWithDetails) (string, error) {
	if details.Template.CalendarID == "" {
		return "", nil
	}

	cal, err := s.repos.Calendar.GetByID(ctx, details.Template.CalendarID)
	if err != nil || cal == nil {
		return "", ErrCalendarNotFound
	}

	switch cal.Provider {
	case models.CalendarProviderGoogle:
		return s.createGoogleEvent(ctx, cal, details)
	case models.CalendarProviderCalDAV, models.CalendarProviderICloud:
		return s.createCalDAVEvent(ctx, cal, details)
	}

	return "", nil
}

// DeleteEvent deletes a calendar event
func (s *CalendarService) DeleteEvent(ctx context.Context, hostID, calendarID, eventID string) error {
	cal, err := s.repos.Calendar.GetByID(ctx, calendarID)
	if err != nil || cal == nil || cal.HostID != hostID {
		return ErrCalendarNotFound
	}

	switch cal.Provider {
	case models.CalendarProviderGoogle:
		return s.deleteGoogleEvent(ctx, cal, eventID)
	case models.CalendarProviderCalDAV, models.CalendarProviderICloud:
		return s.deleteCalDAVEvent(ctx, cal, eventID)
	}

	return nil
}

// GetGoogleAuthURL returns the Google OAuth URL
func (s *CalendarService) GetGoogleAuthURL(state string) string {
	return fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&access_type=offline&prompt=consent&state=%s",
		s.cfg.OAuth.Google.ClientID,
		s.cfg.OAuth.Google.RedirectURL,
		"https://www.googleapis.com/auth/calendar.readonly https://www.googleapis.com/auth/calendar.events",
		state,
	)
}

// Google Calendar API implementations

type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type googleCalendarInfo struct {
	ID   string
	Name string
}

func (s *CalendarService) exchangeGoogleAuthCode(code, redirectURI string) (*googleTokenResponse, error) {
	data := fmt.Sprintf(
		"code=%s&client_id=%s&client_secret=%s&redirect_uri=%s&grant_type=authorization_code",
		code, s.cfg.OAuth.Google.ClientID, s.cfg.OAuth.Google.ClientSecret, redirectURI,
	)

	resp, err := http.Post(
		"https://oauth2.googleapis.com/token",
		"application/x-www-form-urlencoded",
		strings.NewReader(data),
	)
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

	var tokens googleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, err
	}

	return &tokens, nil
}

func (s *CalendarService) refreshGoogleToken(cal *models.CalendarConnection) error {
	if cal.TokenExpiry == nil || time.Now().Before(cal.TokenExpiry.Add(-5*time.Minute)) {
		return nil // Token still valid
	}

	data := fmt.Sprintf(
		"refresh_token=%s&client_id=%s&client_secret=%s&grant_type=refresh_token",
		cal.RefreshToken, s.cfg.OAuth.Google.ClientID, s.cfg.OAuth.Google.ClientSecret,
	)

	resp, err := http.Post(
		"https://oauth2.googleapis.com/token",
		"application/x-www-form-urlencoded",
		strings.NewReader(data),
	)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return ErrCalendarAuth
	}

	var tokens googleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return err
	}

	cal.AccessToken = tokens.AccessToken
	expiry := models.NewSQLiteTime(time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second))
	cal.TokenExpiry = &expiry

	return s.repos.Calendar.Update(context.Background(), cal)
}

func (s *CalendarService) getGoogleCalendarInfo(accessToken string) (*googleCalendarInfo, error) {
	req, _ := http.NewRequest("GET", "https://www.googleapis.com/calendar/v3/calendars/primary", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

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
		return nil, ErrCalendarAuth
	}

	var result struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &googleCalendarInfo{
		ID:   result.ID,
		Name: result.Summary,
	}, nil
}

func (s *CalendarService) getGoogleBusyTimes(ctx context.Context, cal *models.CalendarConnection, start, end time.Time) ([]models.TimeSlot, error) {
	if err := s.refreshGoogleToken(cal); err != nil {
		return nil, err
	}

	// Use freebusy API
	body := fmt.Sprintf(`{
		"timeMin": "%s",
		"timeMax": "%s",
		"items": [{"id": "%s"}]
	}`, start.Format(time.RFC3339), end.Format(time.RFC3339), cal.CalendarID)

	req, _ := http.NewRequest("POST", "https://www.googleapis.com/calendar/v3/freeBusy", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+cal.AccessToken)
	req.Header.Set("Content-Type", "application/json")

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
		return nil, ErrCalendarAuth
	}

	var result struct {
		Calendars map[string]struct {
			Busy []struct {
				Start string `json:"start"`
				End   string `json:"end"`
			} `json:"busy"`
		} `json:"calendars"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var busyTimes []models.TimeSlot
	if calData, ok := result.Calendars[cal.CalendarID]; ok {
		for _, busy := range calData.Busy {
			startTime, _ := time.Parse(time.RFC3339, busy.Start)
			endTime, _ := time.Parse(time.RFC3339, busy.End)
			busyTimes = append(busyTimes, models.TimeSlot{Start: startTime, End: endTime})
		}
	}

	return busyTimes, nil
}

func (s *CalendarService) createGoogleEvent(ctx context.Context, cal *models.CalendarConnection, details *BookingWithDetails) (string, error) {
	if err := s.refreshGoogleToken(cal); err != nil {
		return "", err
	}

	// Build attendees list, filtering out invalid emails
	attendees := []map[string]string{
		{"email": details.Booking.InviteeEmail},
	}
	for _, guest := range details.Booking.AdditionalGuests {
		// Basic email validation - must contain @ and have content on both sides
		if strings.Contains(guest, "@") && len(strings.Split(guest, "@")[0]) > 0 && len(strings.Split(guest, "@")[1]) > 1 {
			attendees = append(attendees, map[string]string{"email": guest})
		}
	}

	event := map[string]interface{}{
		"summary":     details.Template.Name + " with " + details.Booking.InviteeName,
		"description": details.Template.Description,
		"start": map[string]string{
			"dateTime": details.Booking.StartTime.Format(time.RFC3339),
			"timeZone": "UTC",
		},
		"end": map[string]string{
			"dateTime": details.Booking.EndTime.Format(time.RFC3339),
			"timeZone": "UTC",
		},
		"attendees": attendees,
	}

	// Add conference link if available
	if details.Booking.ConferenceLink != "" {
		event["location"] = details.Booking.ConferenceLink
	}

	// Add Google Meet if that's the location type and no link exists
	if details.Template.LocationType == models.ConferencingProviderGoogleMeet && details.Booking.ConferenceLink == "" {
		event["conferenceData"] = map[string]interface{}{
			"createRequest": map[string]string{
				"requestId": details.Booking.ID,
			},
		}
	}

	body, _ := json.Marshal(event)
	url := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events?conferenceDataVersion=1&sendUpdates=all", cal.CalendarID)

	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+cal.AccessToken)
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

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create event: %s", string(respBody))
	}

	var result struct {
		ID             string `json:"id"`
		ConferenceData struct {
			EntryPoints []struct {
				URI string `json:"uri"`
			} `json:"entryPoints"`
		} `json:"conferenceData"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Update conference link if Google Meet was created
	if len(result.ConferenceData.EntryPoints) > 0 && details.Booking.ConferenceLink == "" {
		details.Booking.ConferenceLink = result.ConferenceData.EntryPoints[0].URI
	}

	return result.ID, nil
}

func (s *CalendarService) deleteGoogleEvent(ctx context.Context, cal *models.CalendarConnection, eventID string) error {
	if err := s.refreshGoogleToken(cal); err != nil {
		return err
	}

	url := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s?sendUpdates=all", cal.CalendarID, eventID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer "+cal.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete event")
	}

	return nil
}

// CalDAV implementations (simplified)

func (s *CalendarService) validateCalDAVConnection(url, username, password string) error {
	req, _ := http.NewRequest("OPTIONS", url, nil)
	req.SetBasicAuth(username, password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrCalendarAuth
	}

	return nil
}

func (s *CalendarService) getCalDAVBusyTimes(ctx context.Context, cal *models.CalendarConnection, start, end time.Time) ([]models.TimeSlot, error) {
	// CalDAV freebusy query
	// This is a simplified implementation - production would need full CalDAV support
	query := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<C:free-busy-query xmlns:C="urn:ietf:params:xml:ns:caldav">
  <C:time-range start="%s" end="%s"/>
</C:free-busy-query>`,
		start.Format("20060102T150405Z"),
		end.Format("20060102T150405Z"),
	)

	req, _ := http.NewRequest("REPORT", cal.CalDAVURL, strings.NewReader(query))
	req.SetBasicAuth(cal.CalDAVUsername, cal.CalDAVPassword)
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	// Parse response - simplified, would need proper XML parsing in production
	// For MVP, we'll return empty if the query fails
	return nil, nil
}

func (s *CalendarService) createCalDAVEvent(ctx context.Context, cal *models.CalendarConnection, details *BookingWithDetails) (string, error) {
	eventUID := uuid.New().String()

	// Build iCalendar event
	ics := fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//MeetWhen//EN
BEGIN:VEVENT
UID:%s
DTSTART:%s
DTEND:%s
SUMMARY:%s
DESCRIPTION:%s
ORGANIZER:mailto:%s
ATTENDEE:mailto:%s
END:VEVENT
END:VCALENDAR`,
		eventUID,
		details.Booking.StartTime.UTC().Format("20060102T150405Z"),
		details.Booking.EndTime.UTC().Format("20060102T150405Z"),
		details.Template.Name+" with "+details.Booking.InviteeName,
		details.Template.Description,
		details.Host.Email,
		details.Booking.InviteeEmail,
	)

	eventURL := fmt.Sprintf("%s/%s.ics", strings.TrimSuffix(cal.CalDAVURL, "/"), eventUID)
	req, _ := http.NewRequest("PUT", eventURL, strings.NewReader(ics))
	req.SetBasicAuth(cal.CalDAVUsername, cal.CalDAVPassword)
	req.Header.Set("Content-Type", "text/calendar")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return "", fmt.Errorf("failed to create CalDAV event")
	}

	return eventUID, nil
}

func (s *CalendarService) deleteCalDAVEvent(ctx context.Context, cal *models.CalendarConnection, eventID string) error {
	eventURL := fmt.Sprintf("%s/%s.ics", strings.TrimSuffix(cal.CalDAVURL, "/"), eventID)
	req, _ := http.NewRequest("DELETE", eventURL, nil)
	req.SetBasicAuth(cal.CalDAVUsername, cal.CalDAVPassword)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	return nil
}
