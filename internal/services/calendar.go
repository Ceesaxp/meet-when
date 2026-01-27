package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
	HostID      string
	AuthCode    string
	RedirectURI string
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
	Provider models.CalendarProvider // Optional: defaults to CalendarProviderCalDAV
}

// ConnectCalDAV connects a CalDAV calendar
func (s *CalendarService) ConnectCalDAV(ctx context.Context, input CalDAVConnectInput) (*models.CalendarConnection, error) {
	// Validate CalDAV connection
	if err := s.validateCalDAVConnection(input.URL, input.Username, input.Password); err != nil {
		return nil, fmt.Errorf("failed to validate CalDAV connection: %w", err)
	}

	// Default to CalDAV provider if not specified
	provider := input.Provider
	if provider == "" {
		provider = models.CalendarProviderCalDAV
	}

	now := models.Now()
	calendar := &models.CalendarConnection{
		ID:             uuid.New().String(),
		HostID:         input.HostID,
		Provider:       provider,
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

// GetCalendar returns a single calendar by ID
func (s *CalendarService) GetCalendar(ctx context.Context, calendarID string) (*models.CalendarConnection, error) {
	return s.repos.Calendar.GetByID(ctx, calendarID)
}

// RefreshCalendarSync performs a sync check on a calendar and updates its sync status
func (s *CalendarService) RefreshCalendarSync(ctx context.Context, hostID, calendarID string) error {
	cal, err := s.repos.Calendar.GetByID(ctx, calendarID)
	if err != nil {
		return err
	}
	if cal == nil || cal.HostID != hostID {
		return ErrCalendarNotFound
	}

	// Try to fetch busy times for a short range to test the connection
	start := time.Now()
	end := start.Add(24 * time.Hour)

	var syncErr error
	switch cal.Provider {
	case models.CalendarProviderGoogle:
		_, syncErr = s.getGoogleBusyTimes(ctx, cal, start, end)
	case models.CalendarProviderCalDAV, models.CalendarProviderICloud:
		_, syncErr = s.getCalDAVBusyTimes(ctx, cal, start, end)
	}

	now := models.Now()
	if syncErr != nil {
		// Sync failed - update status
		errMsg := syncErr.Error()
		if errors.Is(syncErr, ErrCalendarAuth) {
			errMsg = "Authentication failed. Please reconnect your calendar."
		}
		return s.repos.Calendar.UpdateSyncStatus(ctx, calendarID, models.CalendarSyncStatusFailed, errMsg, nil)
	}

	// Sync succeeded
	return s.repos.Calendar.UpdateSyncStatus(ctx, calendarID, models.CalendarSyncStatusSynced, "", &now)
}

// UpdateSyncStatus is a helper to update calendar sync status (used by GetBusyTimes tracking)
func (s *CalendarService) UpdateSyncStatus(ctx context.Context, calendarID string, status models.CalendarSyncStatus, syncError string) error {
	var lastSynced *models.SQLiteTime
	if status == models.CalendarSyncStatusSynced {
		now := models.Now()
		lastSynced = &now
	}
	return s.repos.Calendar.UpdateSyncStatus(ctx, calendarID, status, syncError, lastSynced)
}

// SyncCalendar performs a sync check on a calendar and updates its sync status (for background sync)
func (s *CalendarService) SyncCalendar(ctx context.Context, cal *models.CalendarConnection) error {
	// Try to fetch busy times for a short range to test the connection
	start := time.Now()
	end := start.Add(24 * time.Hour)

	var syncErr error
	switch cal.Provider {
	case models.CalendarProviderGoogle:
		_, syncErr = s.getGoogleBusyTimes(ctx, cal, start, end)
	case models.CalendarProviderCalDAV, models.CalendarProviderICloud:
		_, syncErr = s.getCalDAVBusyTimes(ctx, cal, start, end)
	default:
		// Unknown provider - skip
		return nil
	}

	now := models.Now()
	if syncErr != nil {
		// Sync failed - update status
		errMsg := syncErr.Error()
		if errors.Is(syncErr, ErrCalendarAuth) {
			errMsg = "Authentication failed. Please reconnect your calendar."
		}
		return s.repos.Calendar.UpdateSyncStatus(ctx, cal.ID, models.CalendarSyncStatusFailed, errMsg, nil)
	}

	// Sync succeeded
	return s.repos.Calendar.UpdateSyncStatus(ctx, cal.ID, models.CalendarSyncStatusSynced, "", &now)
}

// GetAllCalendars returns all calendar connections (for background sync)
func (s *CalendarService) GetAllCalendars(ctx context.Context) ([]*models.CalendarConnection, error) {
	return s.repos.Calendar.GetAll(ctx)
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
			// Track sync failure
			errMsg := fetchErr.Error()
			if errors.Is(fetchErr, ErrCalendarAuth) {
				errMsg = "Authentication failed. Please reconnect your calendar."
			}
			_ = s.repos.Calendar.UpdateSyncStatus(ctx, cal.ID, models.CalendarSyncStatusFailed, errMsg, nil)
			// Log but continue with other calendars
			log.Printf("Calendar sync failed for %s: %v", cal.ID, fetchErr)
			continue
		}

		// Track sync success
		now := models.Now()
		_ = s.repos.Calendar.UpdateSyncStatus(ctx, cal.ID, models.CalendarSyncStatusSynced, "", &now)

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
	// Use UTC for consistent timezone comparison (TokenExpiry is always stored in UTC)
	if cal.TokenExpiry == nil || time.Now().UTC().Before(cal.TokenExpiry.Add(-5*time.Minute)) {
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

	// Google may return a new refresh token - save it if provided
	if tokens.RefreshToken != "" {
		cal.RefreshToken = tokens.RefreshToken
	}

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

	// Build description with template description and agenda if provided
	description := details.Template.Description
	if details.Booking.Answers != nil {
		if agenda, ok := details.Booking.Answers["agenda"].(string); ok && agenda != "" {
			if description != "" {
				description += "\n\n"
			}
			description += "Agenda:\n" + agenda
		}
	}

	event := map[string]interface{}{
		"summary":     details.Template.Name + " with " + details.Booking.InviteeName,
		"description": description,
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

	// Add location based on location type
	if details.Template.LocationType == models.ConferencingProviderPhone {
		if details.Template.CustomLocation != "" {
			event["location"] = "Call " + details.Template.CustomLocation
		}
	} else if details.Booking.ConferenceLink != "" {
		event["location"] = details.Booking.ConferenceLink
	} else if details.Template.CustomLocation != "" {
		event["location"] = details.Template.CustomLocation
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
	eventsURL := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events?conferenceDataVersion=1&sendUpdates=all", url.PathEscape(cal.CalendarID))

	req, _ := http.NewRequest("POST", eventsURL, strings.NewReader(string(body)))
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

	deleteURL := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s?sendUpdates=all", url.PathEscape(cal.CalendarID), eventID)
	req, _ := http.NewRequest("DELETE", deleteURL, nil)
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
	// Use calendar-query REPORT to fetch VEVENTs in the time range
	// This is more widely supported than free-busy-query, especially on iCloud
	query := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop>
    <D:getetag/>
    <C:calendar-data/>
  </D:prop>
  <C:filter>
    <C:comp-filter name="VCALENDAR">
      <C:comp-filter name="VEVENT">
        <C:time-range start="%s" end="%s"/>
      </C:comp-filter>
    </C:comp-filter>
  </C:filter>
</C:calendar-query>`,
		start.Format("20060102T150405Z"),
		end.Format("20060102T150405Z"),
	)

	req, err := http.NewRequest("REPORT", cal.CalDAVURL, strings.NewReader(query))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(cal.CalDAVUsername, cal.CalDAVPassword)
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CalDAV request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrCalendarAuth
	}

	// Read and parse the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Extract busy times from VCALENDAR data in the response
	busyTimes := parseCalDAVResponse(string(body), start, end)
	return busyTimes, nil
}

// parseCalDAVResponse extracts busy times from CalDAV XML response containing VCALENDAR data
func parseCalDAVResponse(body string, rangeStart, rangeEnd time.Time) []models.TimeSlot {
	var busyTimes []models.TimeSlot

	// Find all calendar-data content (contains ICS data)
	// The response wraps ICS in <cal:calendar-data> or <C:calendar-data> tags
	calDataParts := extractCalendarData(body)

	for _, icsData := range calDataParts {
		events := parseVEvents(icsData, rangeStart, rangeEnd)
		busyTimes = append(busyTimes, events...)
	}

	return busyTimes
}

// extractCalendarData extracts ICS content from CalDAV XML response
func extractCalendarData(xmlBody string) []string {
	var results []string

	// Handle various XML namespace prefixes for calendar-data
	// Common patterns: <cal:calendar-data>, <C:calendar-data>, <calendar-data>
	patterns := []struct {
		start string
		end   string
	}{
		{"<cal:calendar-data>", "</cal:calendar-data>"},
		{"<C:calendar-data>", "</C:calendar-data>"},
		{"<calendar-data>", "</calendar-data>"},
		{"<ns0:calendar-data>", "</ns0:calendar-data>"},
		{"<ns1:calendar-data>", "</ns1:calendar-data>"},
	}

	for _, pattern := range patterns {
		remaining := xmlBody
		for {
			startIdx := strings.Index(remaining, pattern.start)
			if startIdx == -1 {
				break
			}
			remaining = remaining[startIdx+len(pattern.start):]

			endIdx := strings.Index(remaining, pattern.end)
			if endIdx == -1 {
				break
			}

			icsContent := remaining[:endIdx]
			// Unescape XML entities that might be in the ICS data
			icsContent = strings.ReplaceAll(icsContent, "&lt;", "<")
			icsContent = strings.ReplaceAll(icsContent, "&gt;", ">")
			icsContent = strings.ReplaceAll(icsContent, "&amp;", "&")
			icsContent = strings.ReplaceAll(icsContent, "&#13;", "\r")

			results = append(results, icsContent)
			remaining = remaining[endIdx+len(pattern.end):]
		}
	}

	return results
}

// parseVEvents extracts VEVENT start/end times from ICS data
func parseVEvents(icsData string, rangeStart, rangeEnd time.Time) []models.TimeSlot {
	var events []models.TimeSlot

	// Split into lines and unfold (ICS allows long lines to be folded with leading space)
	lines := unfoldICSLines(icsData)

	var inEvent bool
	var eventStart, eventEnd time.Time
	var hasStart, hasEnd bool

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "BEGIN:VEVENT" {
			inEvent = true
			hasStart = false
			hasEnd = false
			eventStart = time.Time{}
			eventEnd = time.Time{}
			continue
		}

		if line == "END:VEVENT" {
			if inEvent && hasStart && hasEnd {
				// Check if event overlaps with our query range
				if eventEnd.After(rangeStart) && eventStart.Before(rangeEnd) {
					events = append(events, models.TimeSlot{
						Start: eventStart,
						End:   eventEnd,
					})
				}
			}
			inEvent = false
			continue
		}

		if !inEvent {
			continue
		}

		// Parse DTSTART
		if strings.HasPrefix(line, "DTSTART") {
			if t, ok := parseICSDateTime(line); ok {
				eventStart = t
				hasStart = true
			}
		}

		// Parse DTEND
		if strings.HasPrefix(line, "DTEND") {
			if t, ok := parseICSDateTime(line); ok {
				eventEnd = t
				hasEnd = true
			}
		}

		// Handle DURATION if DTEND is not present
		if strings.HasPrefix(line, "DURATION") && hasStart && !hasEnd {
			if d, ok := parseICSDuration(line); ok {
				eventEnd = eventStart.Add(d)
				hasEnd = true
			}
		}
	}

	return events
}

// unfoldICSLines handles ICS line folding (continuation lines start with space or tab)
func unfoldICSLines(icsData string) []string {
	// Normalize line endings
	icsData = strings.ReplaceAll(icsData, "\r\n", "\n")
	icsData = strings.ReplaceAll(icsData, "\r", "\n")

	rawLines := strings.Split(icsData, "\n")
	var result []string

	for _, line := range rawLines {
		if len(line) == 0 {
			continue
		}
		// Continuation lines start with space or tab
		if (line[0] == ' ' || line[0] == '\t') && len(result) > 0 {
			// Append to previous line (removing the leading whitespace)
			result[len(result)-1] += line[1:]
		} else {
			result = append(result, line)
		}
	}

	return result
}

// parseICSDateTime parses various ICS date-time formats
// Handles: DTSTART:20240115T100000Z, DTSTART;TZID=America/New_York:20240115T100000
// Also handles VALUE=DATE for all-day events: DTSTART;VALUE=DATE:20240115
func parseICSDateTime(line string) (time.Time, bool) {
	// Split on colon to get the value part
	colonIdx := strings.LastIndex(line, ":")
	if colonIdx == -1 {
		return time.Time{}, false
	}

	value := strings.TrimSpace(line[colonIdx+1:])
	params := line[:colonIdx]

	// Check if it's a date-only value (all-day event)
	isDateOnly := strings.Contains(params, "VALUE=DATE")

	if isDateOnly {
		// Format: YYYYMMDD
		if len(value) >= 8 {
			t, err := time.Parse("20060102", value[:8])
			if err == nil {
				return t, true
			}
		}
		return time.Time{}, false
	}

	// Try UTC format first: YYYYMMDDTHHMMSSZ
	if strings.HasSuffix(value, "Z") {
		t, err := time.Parse("20060102T150405Z", value)
		if err == nil {
			return t, true
		}
	}

	// Try local format: YYYYMMDDTHHMMSS
	// If TZID is specified, we should use it, but for simplicity we'll treat as UTC
	// as busy times are typically converted anyway
	if len(value) >= 15 {
		t, err := time.Parse("20060102T150405", value[:15])
		if err == nil {
			// Check for TZID in params
			if strings.Contains(params, "TZID=") {
				// Extract timezone and try to load it
				tzStart := strings.Index(params, "TZID=")
				if tzStart != -1 {
					tzPart := params[tzStart+5:]
					tzEnd := strings.IndexAny(tzPart, ";:")
					if tzEnd == -1 {
						tzEnd = len(tzPart)
					}
					tzName := tzPart[:tzEnd]
					if loc, err := time.LoadLocation(tzName); err == nil {
						t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
						return t.UTC(), true
					}
				}
			}
			// No timezone info, assume UTC
			return t.UTC(), true
		}
	}

	return time.Time{}, false
}

// parseICSDuration parses ICS DURATION format (RFC 5545)
// Format: DURATION:P1DT2H30M (1 day, 2 hours, 30 minutes)
func parseICSDuration(line string) (time.Duration, bool) {
	colonIdx := strings.LastIndex(line, ":")
	if colonIdx == -1 {
		return 0, false
	}

	value := strings.TrimSpace(line[colonIdx+1:])
	if !strings.HasPrefix(value, "P") {
		return 0, false
	}

	value = value[1:] // Remove P prefix
	var duration time.Duration

	// Handle weeks
	if idx := strings.Index(value, "W"); idx != -1 {
		weeks := 0
		if _, err := fmt.Sscanf(value[:idx], "%d", &weeks); err == nil {
			duration += time.Duration(weeks) * 7 * 24 * time.Hour
		}
		value = value[idx+1:]
	}

	// Handle days
	if idx := strings.Index(value, "D"); idx != -1 {
		days := 0
		if _, err := fmt.Sscanf(value[:idx], "%d", &days); err == nil {
			duration += time.Duration(days) * 24 * time.Hour
		}
		value = value[idx+1:]
	}

	// T separates date and time parts
	value = strings.TrimPrefix(value, "T")

	// Handle hours
	if idx := strings.Index(value, "H"); idx != -1 {
		hours := 0
		if _, err := fmt.Sscanf(value[:idx], "%d", &hours); err == nil {
			duration += time.Duration(hours) * time.Hour
		}
		value = value[idx+1:]
	}

	// Handle minutes
	if idx := strings.Index(value, "M"); idx != -1 {
		mins := 0
		if _, err := fmt.Sscanf(value[:idx], "%d", &mins); err == nil {
			duration += time.Duration(mins) * time.Minute
		}
		value = value[idx+1:]
	}

	// Handle seconds
	if idx := strings.Index(value, "S"); idx != -1 {
		secs := 0
		if _, err := fmt.Sscanf(value[:idx], "%d", &secs); err == nil {
			duration += time.Duration(secs) * time.Second
		}
	}

	return duration, duration > 0
}

func (s *CalendarService) createCalDAVEvent(ctx context.Context, cal *models.CalendarConnection, details *BookingWithDetails) (string, error) {
	eventUID := uuid.New().String()

	// Build location string
	location := ""
	if details.Template.LocationType == models.ConferencingProviderPhone {
		if details.Template.CustomLocation != "" {
			location = "Call " + details.Template.CustomLocation
		}
	} else if details.Booking.ConferenceLink != "" {
		location = details.Booking.ConferenceLink
	} else if details.Template.CustomLocation != "" {
		location = details.Template.CustomLocation
	}

	// Build description with template description and agenda if provided
	description := details.Template.Description
	if details.Booking.Answers != nil {
		if agenda, ok := details.Booking.Answers["agenda"].(string); ok && agenda != "" {
			if description != "" {
				description += "\\n\\n"
			}
			description += "Agenda:\\n" + strings.ReplaceAll(agenda, "\n", "\\n")
		}
	}

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
LOCATION:%s
ORGANIZER:mailto:%s
ATTENDEE:mailto:%s
END:VEVENT
END:VCALENDAR`,
		eventUID,
		details.Booking.StartTime.UTC().Format("20060102T150405Z"),
		details.Booking.EndTime.UTC().Format("20060102T150405Z"),
		details.Template.Name+" with "+details.Booking.InviteeName,
		description,
		location,
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

// AgendaEvent represents a calendar event for the agenda view
type AgendaEvent struct {
	Title        string    `json:"title"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	CalendarName string    `json:"calendar_name"`
	IsAllDay     bool      `json:"is_all_day"`
}

// GetAgendaEvents returns events from all connected calendars for a host within the given time range
func (s *CalendarService) GetAgendaEvents(ctx context.Context, hostID string, startDate, endDate time.Time) ([]AgendaEvent, error) {
	calendars, err := s.repos.Calendar.GetByHostID(ctx, hostID)
	if err != nil {
		return nil, err
	}

	var allEvents []AgendaEvent

	for _, cal := range calendars {
		var events []AgendaEvent
		var fetchErr error

		switch cal.Provider {
		case models.CalendarProviderGoogle:
			events, fetchErr = s.getGoogleAgendaEvents(ctx, cal, startDate, endDate)
		case models.CalendarProviderCalDAV, models.CalendarProviderICloud:
			events, fetchErr = s.getCalDAVAgendaEvents(ctx, cal, startDate, endDate)
		}

		if fetchErr != nil {
			// Log but continue with other calendars (handle errors gracefully)
			log.Printf("Failed to fetch agenda events from calendar %s (%s): %v", cal.Name, cal.ID, fetchErr)
			continue
		}

		allEvents = append(allEvents, events...)
	}

	// Sort events by start time
	sortAgendaEvents(allEvents)

	return allEvents, nil
}

// sortAgendaEvents sorts events by start time
func sortAgendaEvents(events []AgendaEvent) {
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			if events[j].Start.Before(events[i].Start) {
				events[i], events[j] = events[j], events[i]
			}
		}
	}
}

// getGoogleAgendaEvents fetches events from Google Calendar for the agenda view
func (s *CalendarService) getGoogleAgendaEvents(ctx context.Context, cal *models.CalendarConnection, start, end time.Time) ([]AgendaEvent, error) {
	if err := s.refreshGoogleToken(cal); err != nil {
		return nil, err
	}

	// Use events.list API to get full event details
	// CalendarID must be URL-encoded as it often contains @ (email addresses)
	eventsURL := fmt.Sprintf(
		"https://www.googleapis.com/calendar/v3/calendars/%s/events?timeMin=%s&timeMax=%s&singleEvents=true&orderBy=startTime",
		url.PathEscape(cal.CalendarID),
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
	)

	req, _ := http.NewRequest("GET", eventsURL, nil)
	req.Header.Set("Authorization", "Bearer "+cal.AccessToken)

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
		log.Printf("Google Calendar API error (status %d) for calendar %s: %s", resp.StatusCode, cal.CalendarID, string(body))
		return nil, ErrCalendarAuth
	}

	var result struct {
		Items []struct {
			Summary string `json:"summary"`
			Start   struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var events []AgendaEvent
	for _, item := range result.Items {
		var startTime, endTime time.Time
		var isAllDay bool

		// Check if it's an all-day event (uses date instead of dateTime)
		if item.Start.Date != "" {
			isAllDay = true
			startTime, _ = time.Parse("2006-01-02", item.Start.Date)
			endTime, _ = time.Parse("2006-01-02", item.End.Date)
		} else {
			startTime, _ = time.Parse(time.RFC3339, item.Start.DateTime)
			endTime, _ = time.Parse(time.RFC3339, item.End.DateTime)
		}

		events = append(events, AgendaEvent{
			Title:        item.Summary,
			Start:        startTime,
			End:          endTime,
			CalendarName: cal.Name,
			IsAllDay:     isAllDay,
		})
	}

	return events, nil
}

// getCalDAVAgendaEvents fetches events from CalDAV/iCloud for the agenda view
func (s *CalendarService) getCalDAVAgendaEvents(ctx context.Context, cal *models.CalendarConnection, start, end time.Time) ([]AgendaEvent, error) {
	// Use calendar-query REPORT to fetch VEVENTs in the time range
	query := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop>
    <D:getetag/>
    <C:calendar-data/>
  </D:prop>
  <C:filter>
    <C:comp-filter name="VCALENDAR">
      <C:comp-filter name="VEVENT">
        <C:time-range start="%s" end="%s"/>
      </C:comp-filter>
    </C:comp-filter>
  </C:filter>
</C:calendar-query>`,
		start.Format("20060102T150405Z"),
		end.Format("20060102T150405Z"),
	)

	req, err := http.NewRequest("REPORT", cal.CalDAVURL, strings.NewReader(query))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(cal.CalDAVUsername, cal.CalDAVPassword)
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CalDAV request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrCalendarAuth
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Extract agenda events from VCALENDAR data in the response
	events := parseCalDAVAgendaResponse(string(body), cal.Name, start, end)
	return events, nil
}

// parseCalDAVAgendaResponse extracts agenda events from CalDAV XML response containing VCALENDAR data
func parseCalDAVAgendaResponse(body string, calendarName string, rangeStart, rangeEnd time.Time) []AgendaEvent {
	var events []AgendaEvent

	calDataParts := extractCalendarData(body)

	for _, icsData := range calDataParts {
		parsedEvents := parseVEventsForAgenda(icsData, calendarName, rangeStart, rangeEnd)
		events = append(events, parsedEvents...)
	}

	return events
}

// parseVEventsForAgenda extracts VEVENT details for the agenda view from ICS data
func parseVEventsForAgenda(icsData string, calendarName string, rangeStart, rangeEnd time.Time) []AgendaEvent {
	var events []AgendaEvent

	// Split into lines and unfold
	lines := unfoldICSLines(icsData)

	var inEvent bool
	var eventStart, eventEnd time.Time
	var eventTitle string
	var hasStart, hasEnd bool
	var isAllDay bool

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "BEGIN:VEVENT" {
			inEvent = true
			hasStart = false
			hasEnd = false
			isAllDay = false
			eventStart = time.Time{}
			eventEnd = time.Time{}
			eventTitle = ""
			continue
		}

		if line == "END:VEVENT" {
			if inEvent && hasStart && hasEnd {
				// Check if event overlaps with our query range
				if eventEnd.After(rangeStart) && eventStart.Before(rangeEnd) {
					events = append(events, AgendaEvent{
						Title:        eventTitle,
						Start:        eventStart,
						End:          eventEnd,
						CalendarName: calendarName,
						IsAllDay:     isAllDay,
					})
				}
			}
			inEvent = false
			continue
		}

		if !inEvent {
			continue
		}

		// Parse SUMMARY (title)
		if strings.HasPrefix(line, "SUMMARY") {
			colonIdx := strings.Index(line, ":")
			if colonIdx != -1 {
				eventTitle = strings.TrimSpace(line[colonIdx+1:])
			}
		}

		// Parse DTSTART
		if strings.HasPrefix(line, "DTSTART") {
			// Check if it's an all-day event
			if strings.Contains(line, "VALUE=DATE") && !strings.Contains(line, "VALUE=DATE-TIME") {
				isAllDay = true
			}
			if t, ok := parseICSDateTime(line); ok {
				eventStart = t
				hasStart = true
			}
		}

		// Parse DTEND
		if strings.HasPrefix(line, "DTEND") {
			if t, ok := parseICSDateTime(line); ok {
				eventEnd = t
				hasEnd = true
			}
		}

		// Handle DURATION if DTEND is not present
		if strings.HasPrefix(line, "DURATION") && hasStart && !hasEnd {
			if d, ok := parseICSDuration(line); ok {
				eventEnd = eventStart.Add(d)
				hasEnd = true
			}
		}
	}

	return events
}
