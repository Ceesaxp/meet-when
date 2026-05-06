package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

var (
	ErrBookingNotFound    = errors.New("booking not found")
	ErrSlotNotAvailable   = errors.New("selected time slot is no longer available")
	ErrInvalidBookingTime = errors.New("invalid booking time")
	ErrBookingCancelled   = errors.New("booking has already been cancelled")
)

// calculateSmartDuration adjusts a meeting duration to end early, giving buffer time.
// Durations <= 30 min subtract 5 min; durations > 30 min subtract 10 min.
// Minimum resulting duration is 5 minutes.
func calculateSmartDuration(duration int) int {
	if duration <= 30 {
		duration -= 5
	} else {
		duration -= 10
	}
	if duration < 5 {
		duration = 5
	}
	return duration
}

// BookingService handles booking operations
type BookingService struct {
	cfg          *config.Config
	repos        *repository.Repositories
	calendar     *CalendarService
	syncer       *CalendarEventSyncer
	conferencing *ConferencingService
	email        *EmailService
	auditLog     *AuditLogService
	contact      *ContactService
}

// NewBookingService creates a new booking service
func NewBookingService(
	cfg *config.Config,
	repos *repository.Repositories,
	calendar *CalendarService,
	syncer *CalendarEventSyncer,
	conferencing *ConferencingService,
	email *EmailService,
	auditLog *AuditLogService,
	contact *ContactService,
) *BookingService {
	return &BookingService{
		cfg:          cfg,
		repos:        repos,
		calendar:     calendar,
		syncer:       syncer,
		conferencing: conferencing,
		email:        email,
		auditLog:     auditLog,
		contact:      contact,
	}
}

// CreateBookingInput represents the input for creating a booking
type CreateBookingInput struct {
	TemplateID       string
	HostID           string
	TenantID         string
	StartTime        time.Time
	Duration         int // minutes
	InviteeName      string
	InviteeEmail     string
	InviteeTimezone  string
	InviteePhone     string
	AdditionalGuests []string
	Answers          models.JSONMap
}

// BookingWithDetails includes booking with related entities
type BookingWithDetails struct {
	Booking  *models.Booking
	Template *models.MeetingTemplate
	Host     *models.Host
	Tenant   *models.Tenant
}

// CreateBooking creates a new booking request
func (s *BookingService) CreateBooking(ctx context.Context, input CreateBookingInput) (*BookingWithDetails, error) {
	// Validate template exists and is active
	template, err := s.repos.Template.GetByID(ctx, input.TemplateID)
	if err != nil || template == nil || !template.IsActive {
		return nil, ErrTemplateNotFound
	}

	// Validate duration is allowed
	validDuration := false
	for _, d := range template.Durations {
		if d == input.Duration {
			validDuration = true
			break
		}
	}
	if !validDuration && len(template.Durations) > 0 {
		input.Duration = template.Durations[0]
	}

	// Get host and tenant
	host, _ := s.repos.Host.GetByID(ctx, input.HostID)
	tenant, _ := s.repos.Tenant.GetByID(ctx, input.TenantID)

	// Calculate end time (apply smart duration if enabled)
	actualDuration := input.Duration
	if host != nil && host.SmartDurations {
		actualDuration = calculateSmartDuration(input.Duration)
	}
	endTime := input.StartTime.Add(time.Duration(actualDuration) * time.Minute)

	// Validate time is in the future with minimum notice
	minNotice := time.Duration(template.MinNoticeMinutes) * time.Minute
	if input.StartTime.Before(time.Now().Add(minNotice)) {
		return nil, ErrInvalidBookingTime
	}

	// TODO: Validate slot is still available (race condition check)
	// For MVP, we'll rely on the calendar event creation to catch conflicts

	// Generate booking token
	token, err := generateToken(32)
	if err != nil {
		return nil, err
	}

	// Determine initial status
	status := models.BookingStatusPending
	if !template.RequiresApproval {
		status = models.BookingStatusConfirmed
	}

	now := models.Now()
	booking := &models.Booking{
		ID:               uuid.New().String(),
		TemplateID:       input.TemplateID,
		HostID:           input.HostID,
		Token:            token,
		Status:           status,
		StartTime:        models.NewSQLiteTime(input.StartTime.UTC()),
		EndTime:          models.NewSQLiteTime(endTime.UTC()),
		Duration:         input.Duration,
		InviteeName:      input.InviteeName,
		InviteeEmail:     input.InviteeEmail,
		InviteeTimezone:  input.InviteeTimezone,
		InviteePhone:     input.InviteePhone,
		AdditionalGuests: input.AdditionalGuests,
		Answers:          input.Answers,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := s.repos.Booking.Create(ctx, booking); err != nil {
		return nil, err
	}

	details := &BookingWithDetails{
		Booking:  booking,
		Template: template,
		Host:     host,
		Tenant:   tenant,
	}

	// If auto-approved, create calendar event and send confirmation
	if status == models.BookingStatusConfirmed {
		if err := s.processConfirmedBooking(ctx, details); err != nil {
			log.Printf("[BOOKING] Warning: processConfirmedBooking failed (booking still created): %v", err)
		}
	} else {
		// Send pending notification to host
		s.email.SendBookingRequested(ctx, details)
	}

	// Audit log
	s.auditLog.Log(ctx, input.TenantID, nil, "booking.created", "booking", booking.ID, models.JSONMap{
		"invitee_email": input.InviteeEmail,
		"status":        string(status),
	}, "")

	return details, nil
}

// ApproveBooking approves a pending booking
func (s *BookingService) ApproveBooking(ctx context.Context, hostID, tenantID, bookingID string) (*BookingWithDetails, error) {
	booking, err := s.repos.Booking.GetByID(ctx, bookingID)
	if err != nil || booking == nil || booking.HostID != hostID {
		return nil, ErrBookingNotFound
	}

	if booking.Status != models.BookingStatusPending {
		return nil, errors.New("booking is not pending approval")
	}

	booking.Status = models.BookingStatusConfirmed

	if err := s.repos.Booking.Update(ctx, booking); err != nil {
		return nil, err
	}

	// Get related entities
	template, _ := s.repos.Template.GetByID(ctx, booking.TemplateID)
	host, _ := s.repos.Host.GetByID(ctx, hostID)
	tenant, _ := s.repos.Tenant.GetByID(ctx, tenantID)

	details := &BookingWithDetails{
		Booking:  booking,
		Template: template,
		Host:     host,
		Tenant:   tenant,
	}

	// Process confirmed booking (create calendar event, send emails)
	if err := s.processConfirmedBooking(ctx, details); err != nil {
		log.Printf("[BOOKING] Warning: processConfirmedBooking failed after approval: %v", err)
	}

	// Audit log
	s.auditLog.Log(ctx, tenantID, &hostID, "booking.approved", "booking", bookingID, nil, "")

	return details, nil
}

// RejectBooking rejects a pending booking
func (s *BookingService) RejectBooking(ctx context.Context, hostID, tenantID, bookingID, reason string) error {
	booking, err := s.repos.Booking.GetByID(ctx, bookingID)
	if err != nil || booking == nil || booking.HostID != hostID {
		return ErrBookingNotFound
	}

	if booking.Status != models.BookingStatusPending {
		return errors.New("booking is not pending")
	}

	booking.Status = models.BookingStatusRejected
	booking.CancelledBy = "host"
	booking.CancelReason = reason

	if err := s.repos.Booking.Update(ctx, booking); err != nil {
		return err
	}

	// Get related entities and send rejection email
	template, _ := s.repos.Template.GetByID(ctx, booking.TemplateID)
	host, _ := s.repos.Host.GetByID(ctx, hostID)
	tenant, _ := s.repos.Tenant.GetByID(ctx, tenantID)

	details := &BookingWithDetails{
		Booking:  booking,
		Template: template,
		Host:     host,
		Tenant:   tenant,
	}

	s.email.SendBookingRejected(ctx, details)

	// Audit log
	s.auditLog.Log(ctx, tenantID, &hostID, "booking.rejected", "booking", bookingID, models.JSONMap{
		"reason": reason,
	}, "")

	return nil
}

// CancelBooking cancels a booking (by host or invitee)
func (s *BookingService) CancelBooking(ctx context.Context, bookingID, cancelledBy, reason string) error {
	booking, err := s.repos.Booking.GetByID(ctx, bookingID)
	if err != nil || booking == nil {
		return ErrBookingNotFound
	}

	if booking.Status == models.BookingStatusCancelled {
		return ErrBookingCancelled
	}

	booking.Status = models.BookingStatusCancelled
	booking.CancelledBy = cancelledBy
	booking.CancelReason = reason

	if err := s.repos.Booking.Update(ctx, booking); err != nil {
		return err
	}

	// Delete calendar events for all pooled hosts
	s.deleteAllCalendarEvents(ctx, booking)

	// Get related entities and send cancellation email
	template, _ := s.repos.Template.GetByID(ctx, booking.TemplateID)
	host, _ := s.repos.Host.GetByID(ctx, booking.HostID)
	tenant, _ := s.repos.Tenant.GetByID(ctx, host.TenantID)

	details := &BookingWithDetails{
		Booking:  booking,
		Template: template,
		Host:     host,
		Tenant:   tenant,
	}

	s.email.SendBookingCancelled(ctx, details)

	// Audit log
	s.auditLog.Log(ctx, tenant.ID, nil, "booking.cancelled", "booking", bookingID, models.JSONMap{
		"cancelled_by": cancelledBy,
		"reason":       reason,
	}, "")

	return nil
}

// deleteAllCalendarEvents deletes calendar events for all hosts. Tracked rows
// (pooled and post-refactor single-host) flow through the syncer; pre-refactor
// bookings that only have booking.CalendarEventID + template.CalendarID still
// fall back to a direct DeleteEvent call.
func (s *BookingService) deleteAllCalendarEvents(ctx context.Context, booking *models.Booking) {
	deleted, err := s.syncer.Delete(ctx, ItemKindBooking, booking.ID)
	if err != nil {
		log.Printf("[BOOKING] Error during syncer delete for booking %s: %v", booking.ID, err)
	}
	if deleted > 0 {
		return
	}
	if booking.CalendarEventID == "" {
		return
	}
	template, _ := s.repos.Template.GetByID(ctx, booking.TemplateID)
	if template == nil {
		return
	}
	if err := s.calendar.DeleteEvent(ctx, booking.HostID, template.CalendarID, booking.CalendarEventID); err != nil {
		log.Printf("[BOOKING] Error deleting legacy calendar event: %v", err)
	}
}

// UpdateBookingInput captures the host-editable fields of a confirmed booking.
// Each pointer is nil if that field is not being changed; non-nil means
// "replace with this value". RegenerateConferenceLink is a separate one-shot
// action: when true the existing link is replaced by a freshly created one
// using the template's configured provider (used to recover after a Zoom
// reauth — see issue #38).
type UpdateBookingInput struct {
	HostID                   string
	TenantID                 string
	BookingID                string
	HostNotes                *string
	AdditionalGuests         *[]string
	ConferenceLink           *string
	RegenerateConferenceLink bool
}

// UpdateBooking applies host edits to an existing confirmed booking and
// distributes an updated calendar invitation. Returns the (possibly modified)
// list of fields that actually changed so the audit log and email reflect the
// real diff. ErrConferencingReauthRequired is returned untouched if the host
// asked to regenerate a link but the conferencing token is dead — the caller
// should surface a "reconnect first" message.
func (s *BookingService) UpdateBooking(ctx context.Context, input UpdateBookingInput) (*BookingWithDetails, []string, error) {
	booking, err := s.repos.Booking.GetByID(ctx, input.BookingID)
	if err != nil || booking == nil || booking.HostID != input.HostID {
		return nil, nil, ErrBookingNotFound
	}
	if booking.Status != models.BookingStatusConfirmed {
		return nil, nil, errors.New("only confirmed bookings can be edited")
	}

	template, err := s.repos.Template.GetByID(ctx, booking.TemplateID)
	if err != nil || template == nil {
		return nil, nil, ErrTemplateNotFound
	}
	host, _ := s.repos.Host.GetByID(ctx, booking.HostID)
	tenant, _ := s.repos.Tenant.GetByID(ctx, input.TenantID)
	details := &BookingWithDetails{
		Booking:  booking,
		Template: template,
		Host:     host,
		Tenant:   tenant,
	}

	var changed []string

	if input.HostNotes != nil {
		if booking.Answers == nil {
			booking.Answers = models.JSONMap{}
		}
		old, _ := booking.Answers["host_notes"].(string)
		if old != *input.HostNotes {
			if *input.HostNotes == "" {
				delete(booking.Answers, "host_notes")
			} else {
				booking.Answers["host_notes"] = *input.HostNotes
			}
			changed = append(changed, "notes")
		}
	}

	if input.AdditionalGuests != nil {
		oldList := append([]string(nil), booking.AdditionalGuests...)
		newList := normalizeGuestList(*input.AdditionalGuests)
		if !sameStringSlice(oldList, newList) {
			booking.AdditionalGuests = newList
			changed = append(changed, "additional guests")
		}
	}

	if input.RegenerateConferenceLink {
		// Clear existing link and re-create using the template's provider.
		// On reauth-required, we surface the typed error to the handler so the
		// UI can prompt the host to reconnect — booking state is left intact.
		link, err := s.conferencing.CreateMeeting(ctx, details)
		if err != nil {
			if errors.Is(err, ErrConferencingReauthRequired) {
				return details, nil, err
			}
			return details, nil, fmt.Errorf("regenerate conference link: %w", err)
		}
		if link != "" && link != booking.ConferenceLink {
			booking.ConferenceLink = link
			changed = append(changed, "conference link")
			// Successful regeneration clears any prior _conference_error marker.
			if booking.Answers != nil {
				delete(booking.Answers, "_conference_error")
			}
		}
	} else if input.ConferenceLink != nil && *input.ConferenceLink != booking.ConferenceLink {
		booking.ConferenceLink = *input.ConferenceLink
		changed = append(changed, "conference link")
		if booking.Answers != nil {
			delete(booking.Answers, "_conference_error")
		}
	}

	if len(changed) == 0 {
		return details, nil, nil
	}

	booking.UpdatedAt = models.Now()
	if err := s.repos.Booking.Update(ctx, booking); err != nil {
		return details, nil, err
	}

	// Push the changes to the host's calendar so attendees get an updated
	// invitation. We try the legacy single-host event first, then fall through
	// to any pooled-host events so all calendars stay in sync.
	s.updateAllCalendarEvents(ctx, details, template)

	if tenant != nil && host != nil {
		s.email.SendBookingUpdated(ctx, details, changed)
	}

	hostID := input.HostID
	tenantID := input.TenantID
	s.auditLog.Log(ctx, tenantID, &hostID, "booking.updated", "booking", booking.ID, models.JSONMap{
		"changed_fields": changed,
	}, "")

	return details, changed, nil
}

// updateAllCalendarEvents patches the calendar event(s) for a booking after a
// host edit. Tracked rows flow through the syncer (which preserves the
// per-host ID and writes back any replacement ID returned by the provider).
// For pre-refactor bookings that only have booking.CalendarEventID +
// template.CalendarID, falls back to a direct UpdateEvent call.
func (s *BookingService) updateAllCalendarEvents(ctx context.Context, details *BookingWithDetails, template *models.MeetingTemplate) {
	rows, err := s.repos.BookingCalendarEvent.GetByBookingID(ctx, details.Booking.ID)
	if err != nil {
		log.Printf("[BOOKING] Error loading calendar events for booking %s: %v", details.Booking.ID, err)
	}

	if len(rows) > 0 {
		input := s.calendar.BuildCalendarEventInputForBooking(details)
		req := CalendarSyncRequest{
			Kind:   ItemKindBooking,
			ItemID: details.Booking.ID,
			Input:  *input,
		}
		if err := s.syncer.Update(ctx, req); err != nil {
			log.Printf("[BOOKING] Error during syncer update for booking %s: %v", details.Booking.ID, err)
		}
		return
	}

	if details.Booking.CalendarEventID != "" && template.CalendarID != "" {
		input := s.calendar.BuildCalendarEventInputForBooking(details)
		newID, err := s.calendar.UpdateEvent(ctx, template.CalendarID, input)
		if err != nil {
			log.Printf("[BOOKING] Error updating legacy calendar event: %v", err)
			return
		}
		if newID != "" && newID != details.Booking.CalendarEventID {
			details.Booking.CalendarEventID = newID
			if err := s.repos.Booking.Update(ctx, details.Booking); err != nil {
				log.Printf("[BOOKING] Error persisting new event ID: %v", err)
			}
		}
	}
}

func normalizeGuestList(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, g := range in {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if !strings.Contains(g, "@") {
			continue
		}
		key := strings.ToLower(g)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, g)
	}
	return out
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// CancelBookingByToken cancels a booking using the public token (for invitee cancellation)
func (s *BookingService) CancelBookingByToken(ctx context.Context, token, reason string) error {
	booking, err := s.repos.Booking.GetByToken(ctx, token)
	if err != nil || booking == nil {
		return ErrBookingNotFound
	}

	return s.CancelBooking(ctx, booking.ID, "invitee", reason)
}

// GetBookingByToken retrieves a booking by its public token
func (s *BookingService) GetBookingByToken(ctx context.Context, token string) (*BookingWithDetails, error) {
	booking, err := s.repos.Booking.GetByToken(ctx, token)
	if err != nil || booking == nil {
		return nil, ErrBookingNotFound
	}

	template, _ := s.repos.Template.GetByID(ctx, booking.TemplateID)
	host, _ := s.repos.Host.GetByID(ctx, booking.HostID)
	tenant, _ := s.repos.Tenant.GetByID(ctx, host.TenantID)

	return &BookingWithDetails{
		Booking:  booking,
		Template: template,
		Host:     host,
		Tenant:   tenant,
	}, nil
}

// GetBooking retrieves a single booking by ID
func (s *BookingService) GetBooking(ctx context.Context, bookingID string) (*models.Booking, error) {
	return s.repos.Booking.GetByID(ctx, bookingID)
}

// GetBookings retrieves bookings for a host
func (s *BookingService) GetBookings(ctx context.Context, hostID string, status *models.BookingStatus, includeArchived bool) ([]*models.Booking, error) {
	return s.repos.Booking.GetByHostID(ctx, hostID, status, includeArchived)
}

// GetPendingBookings retrieves pending bookings for a host
func (s *BookingService) GetPendingBookings(ctx context.Context, hostID string) ([]*models.Booking, error) {
	status := models.BookingStatusPending
	return s.repos.Booking.GetByHostID(ctx, hostID, &status, false)
}

// RescheduleBookingInput represents the input for rescheduling a booking
type RescheduleBookingInput struct {
	Token        string
	NewStartTime time.Time
	NewDuration  int
}

// RescheduleBooking reschedules an existing booking to a new time
func (s *BookingService) RescheduleBooking(ctx context.Context, input RescheduleBookingInput) (*BookingWithDetails, time.Time, error) {
	// Get the existing booking by token
	oldBooking, err := s.repos.Booking.GetByToken(ctx, input.Token)
	if err != nil || oldBooking == nil {
		return nil, time.Time{}, ErrBookingNotFound
	}

	// Can't reschedule a cancelled or rejected booking
	if oldBooking.Status == models.BookingStatusCancelled || oldBooking.Status == models.BookingStatusRejected {
		return nil, time.Time{}, ErrBookingCancelled
	}

	// Store old start time for email notification
	oldStartTime := oldBooking.StartTime.Time

	// Get template to validate duration and get settings
	template, err := s.repos.Template.GetByID(ctx, oldBooking.TemplateID)
	if err != nil || template == nil {
		return nil, time.Time{}, ErrTemplateNotFound
	}

	// Validate duration against allowed template durations
	validDuration := false
	for _, d := range template.Durations {
		if d == input.NewDuration {
			validDuration = true
			break
		}
	}
	if !validDuration && len(template.Durations) > 0 {
		input.NewDuration = template.Durations[0]
	}

	// Get host and tenant
	host, _ := s.repos.Host.GetByID(ctx, oldBooking.HostID)
	if host == nil {
		return nil, time.Time{}, fmt.Errorf("host not found: %s", oldBooking.HostID)
	}
	tenant, _ := s.repos.Tenant.GetByID(ctx, host.TenantID)

	// Calculate new end time (apply smart duration if enabled)
	actualDuration := input.NewDuration
	if host.SmartDurations {
		actualDuration = calculateSmartDuration(input.NewDuration)
	}
	newEndTime := input.NewStartTime.Add(time.Duration(actualDuration) * time.Minute)

	// Validate time is in the future with minimum notice
	minNotice := time.Duration(template.MinNoticeMinutes) * time.Minute
	if input.NewStartTime.Before(time.Now().Add(minNotice)) {
		return nil, time.Time{}, ErrInvalidBookingTime
	}

	// Delete all old calendar events (pooled and legacy)
	s.deleteAllCalendarEvents(ctx, oldBooking)

	// Update booking with new times
	oldBooking.StartTime = models.NewSQLiteTime(input.NewStartTime.UTC())
	oldBooking.EndTime = models.NewSQLiteTime(newEndTime.UTC())
	oldBooking.Duration = input.NewDuration
	oldBooking.UpdatedAt = models.Now()
	oldBooking.CalendarEventID = "" // Will be set when new event is created
	oldBooking.ConferenceLink = ""  // Will be regenerated if needed

	if err := s.repos.Booking.Update(ctx, oldBooking); err != nil {
		return nil, time.Time{}, err
	}

	details := &BookingWithDetails{
		Booking:  oldBooking,
		Template: template,
		Host:     host,
		Tenant:   tenant,
	}

	// If booking was confirmed, recreate calendar event and conference link
	if oldBooking.Status == models.BookingStatusConfirmed {
		// Create new conference link if needed
		if template.LocationType == models.ConferencingProviderGoogleMeet ||
			template.LocationType == models.ConferencingProviderZoom {
			link, err := s.conferencing.CreateMeeting(ctx, details)
			if err != nil {
				log.Printf("[RESCHEDULE] Error creating conference link: %v", err)
			} else if link != "" {
				details.Booking.ConferenceLink = link
			}
		}

		hosts := s.bookingHostTargets(ctx, details)
		input := s.calendar.BuildCalendarEventInputForBooking(details)
		firstID, conferenceLink, err := s.syncer.Create(ctx, CalendarSyncRequest{
			Kind:   ItemKindBooking,
			ItemID: details.Booking.ID,
			Input:  *input,
			Hosts:  hosts,
		})
		if err != nil {
			log.Printf("[RESCHEDULE] Error during syncer create: %v", err)
		}
		if firstID != "" {
			details.Booking.CalendarEventID = firstID
		}
		if conferenceLink != "" && details.Booking.ConferenceLink == "" {
			details.Booking.ConferenceLink = conferenceLink
		}

		// Update booking with new conference link and event ID
		if err := s.repos.Booking.Update(ctx, details.Booking); err != nil {
			log.Printf("[RESCHEDULE] Error updating booking: %v", err)
		}
	}

	// Audit log
	s.auditLog.Log(ctx, tenant.ID, nil, "booking.rescheduled", "booking", oldBooking.ID, models.JSONMap{
		"old_start_time": oldStartTime.Format(time.RFC3339),
		"new_start_time": input.NewStartTime.Format(time.RFC3339),
	}, "")

	return details, oldStartTime, nil
}

// GetBookingCountsByHostID returns booking counts grouped by template for a given host
func (s *BookingService) GetBookingCountsByHostID(ctx context.Context, hostID string) (map[string]*repository.BookingCount, error) {
	return s.repos.Booking.GetBookingCountsByHostID(ctx, hostID)
}

// ArchiveBooking archives a past booking regardless of status
func (s *BookingService) ArchiveBooking(ctx context.Context, hostID, tenantID, bookingID string) error {
	booking, err := s.repos.Booking.GetByID(ctx, bookingID)
	if err != nil || booking == nil || booking.HostID != hostID {
		return ErrBookingNotFound
	}

	// Allow archiving any past booking; block future confirmed/pending bookings
	isPast := booking.EndTime.Before(time.Now())
	if !isPast && (booking.Status == models.BookingStatusConfirmed || booking.Status == models.BookingStatusPending) {
		return errors.New("future confirmed or pending bookings cannot be archived")
	}

	booking.IsArchived = true

	if err := s.repos.Booking.Update(ctx, booking); err != nil {
		return err
	}

	// Audit log
	s.auditLog.Log(ctx, tenantID, &hostID, "booking.archived", "booking", bookingID, nil, "")

	return nil
}

// UnarchiveBooking restores an archived booking
func (s *BookingService) UnarchiveBooking(ctx context.Context, hostID, tenantID, bookingID string) error {
	booking, err := s.repos.Booking.GetByID(ctx, bookingID)
	if err != nil || booking == nil || booking.HostID != hostID {
		return ErrBookingNotFound
	}

	if !booking.IsArchived {
		return errors.New("booking is not archived")
	}

	booking.IsArchived = false

	if err := s.repos.Booking.Update(ctx, booking); err != nil {
		return err
	}

	// Audit log
	s.auditLog.Log(ctx, tenantID, &hostID, "booking.unarchived", "booking", bookingID, nil, "")

	return nil
}

// BulkArchiveBookings archives all cancelled and rejected bookings for a host
func (s *BookingService) BulkArchiveBookings(ctx context.Context, hostID, tenantID string) (int, error) {
	// Get all non-archived bookings
	bookings, err := s.repos.Booking.GetByHostID(ctx, hostID, nil, false)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, booking := range bookings {
		// Only archive cancelled or rejected bookings
		if (booking.Status == models.BookingStatusCancelled || booking.Status == models.BookingStatusRejected) && !booking.IsArchived {
			booking.IsArchived = true
			if err := s.repos.Booking.Update(ctx, booking); err != nil {
				log.Printf("[BOOKING] Failed to archive booking %s: %v", booking.ID, err)
				continue
			}
			count++
		}
	}

	// Audit log
	if count > 0 {
		s.auditLog.Log(ctx, tenantID, &hostID, "booking.bulk_archived", "booking", "", models.JSONMap{
			"count": count,
		}, "")
	}

	return count, nil
}

// BulkArchivePastBookings archives all past bookings for a host regardless of status.
// Uses the repository's ArchiveOldBookingsByHostID with time.Now() as cutoff.
func (s *BookingService) BulkArchivePastBookings(ctx context.Context, hostID, tenantID string) (int, error) {
	count, err := s.repos.Booking.ArchiveOldBookingsByHostID(ctx, hostID, time.Now())
	if err != nil {
		return 0, err
	}

	if count > 0 {
		s.auditLog.Log(ctx, tenantID, &hostID, "booking.bulk_archived_past", "booking", "", models.JSONMap{
			"count": count,
		}, "")
	}

	return count, nil
}

// RetryCalendarEvent retries calendar event creation for a confirmed booking
// that has no calendar event (e.g., due to expired token at booking time).
// Unlike processConfirmedBooking, this does NOT re-send emails or re-create conference links.
func (s *BookingService) RetryCalendarEvent(ctx context.Context, hostID, tenantID, bookingID string) error {
	booking, err := s.repos.Booking.GetByID(ctx, bookingID)
	if err != nil || booking == nil || booking.HostID != hostID {
		return ErrBookingNotFound
	}

	if booking.Status != models.BookingStatusConfirmed {
		return fmt.Errorf("booking is not confirmed")
	}

	if booking.CalendarEventID != "" {
		return fmt.Errorf("calendar event already exists")
	}

	template, err := s.repos.Template.GetByID(ctx, booking.TemplateID)
	if err != nil || template == nil {
		return ErrTemplateNotFound
	}

	host, _ := s.repos.Host.GetByID(ctx, hostID)
	tenant, _ := s.repos.Tenant.GetByID(ctx, tenantID)

	details := &BookingWithDetails{
		Booking:  booking,
		Template: template,
		Host:     host,
		Tenant:   tenant,
	}

	hosts := s.bookingHostTargets(ctx, details)
	input := s.calendar.BuildCalendarEventInputForBooking(details)
	firstID, conferenceLink, err := s.syncer.Create(ctx, CalendarSyncRequest{
		Kind:   ItemKindBooking,
		ItemID: details.Booking.ID,
		Input:  *input,
		Hosts:  hosts,
	})
	if err != nil {
		return fmt.Errorf("failed to create calendar event: %w", err)
	}
	if firstID != "" {
		details.Booking.CalendarEventID = firstID
	}
	if conferenceLink != "" && details.Booking.ConferenceLink == "" {
		details.Booking.ConferenceLink = conferenceLink
	}

	// Update booking with the new event ID
	if err := s.repos.Booking.Update(ctx, details.Booking); err != nil {
		return fmt.Errorf("failed to update booking: %w", err)
	}

	// Audit log
	s.auditLog.Log(ctx, tenantID, &hostID, "booking.calendar_retry", "booking", bookingID, nil, "")

	return nil
}

// BulkRetryCalendarEvents retries calendar event creation for all confirmed bookings
// with no calendar event ID. Returns success count and fail count.
func (s *BookingService) BulkRetryCalendarEvents(ctx context.Context, hostID, tenantID string) (int, int, error) {
	confirmedStatus := models.BookingStatusConfirmed
	bookings, err := s.repos.Booking.GetByHostID(ctx, hostID, &confirmedStatus, false)
	if err != nil {
		return 0, 0, err
	}

	successCount := 0
	failCount := 0

	for _, booking := range bookings {
		if booking.CalendarEventID != "" {
			continue // Already has a calendar event
		}

		// Check that the template has a calendar configured
		template, err := s.repos.Template.GetByID(ctx, booking.TemplateID)
		if err != nil || template == nil || template.CalendarID == "" {
			continue // No calendar configured for this template
		}

		if err := s.RetryCalendarEvent(ctx, hostID, tenantID, booking.ID); err != nil {
			log.Printf("[BOOKING] Failed to retry calendar event for booking %s: %v", booking.ID, err)
			failCount++
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		s.auditLog.Log(ctx, tenantID, &hostID, "booking.bulk_calendar_retry", "booking", "", models.JSONMap{
			"success_count": successCount,
			"fail_count":    failCount,
		}, "")
	}

	return successCount, failCount, nil
}

// processConfirmedBooking handles post-confirmation actions
func (s *BookingService) processConfirmedBooking(ctx context.Context, details *BookingWithDetails) error {
	log.Printf("[BOOKING] processConfirmedBooking: booking=%s template=%s calendar=%s",
		details.Booking.ID, details.Template.ID, details.Template.CalendarID)

	// Create conference link if needed (use owner's credentials)
	if details.Template.LocationType == models.ConferencingProviderGoogleMeet ||
		details.Template.LocationType == models.ConferencingProviderZoom {
		log.Printf("[BOOKING] Creating conference link for location type: %s", details.Template.LocationType)
		link, err := s.conferencing.CreateMeeting(ctx, details)
		if err != nil {
			log.Printf("[BOOKING] Error creating conference link: %v", err)
			s.recordConferencingFailure(ctx, details, err)
		} else if link != "" {
			log.Printf("[BOOKING] Conference link created: %s", link)
			details.Booking.ConferenceLink = link
		}
	}

	hosts := s.bookingHostTargets(ctx, details)
	input := s.calendar.BuildCalendarEventInputForBooking(details)
	firstID, conferenceLink, err := s.syncer.Create(ctx, CalendarSyncRequest{
		Kind:   ItemKindBooking,
		ItemID: details.Booking.ID,
		Input:  *input,
		Hosts:  hosts,
	})
	if err != nil {
		log.Printf("[BOOKING] Error during syncer create for booking %s: %v", details.Booking.ID, err)
	}
	if firstID != "" && details.Booking.CalendarEventID == "" {
		details.Booking.CalendarEventID = firstID
	}
	if conferenceLink != "" && details.Booking.ConferenceLink == "" {
		details.Booking.ConferenceLink = conferenceLink
	}

	// Update booking with conference link and event ID
	if err := s.repos.Booking.Update(ctx, details.Booking); err != nil {
		log.Printf("[BOOKING] Error updating booking: %v", err)
		return err
	}

	// Send confirmation emails
	s.email.SendBookingConfirmed(ctx, details)

	// Upsert contact from confirmed booking (errors are logged, not propagated)
	s.contact.UpsertFromBooking(ctx, details)

	return nil
}

// bookingHostTargets resolves the calendar fan-out targets for a booking:
// pooled hosts when the template has any (owner first, siblings after), or
// just the booking host when the template is single-host. Each target's
// CalendarID is left empty when the syncer should resolve it from the host's
// default + first writable fallback.
func (s *BookingService) bookingHostTargets(ctx context.Context, details *BookingWithDetails) []HostTarget {
	pooled, err := s.repos.TemplateHost.GetByTemplateIDWithHost(ctx, details.Template.ID)
	if err != nil {
		log.Printf("[BOOKING] Error loading pooled hosts: %v", err)
		pooled = nil
	}
	if len(pooled) == 0 {
		// Single-host fallback: write to the template's configured calendar.
		return []HostTarget{{
			HostID:     details.Booking.HostID,
			CalendarID: details.Template.CalendarID,
			IsOwner:    true,
		}}
	}
	targets := make([]HostTarget, 0, len(pooled))
	for _, th := range pooled {
		targets = append(targets, HostTarget{
			HostID:  th.HostID,
			IsOwner: th.Role == models.TemplateHostRoleOwner,
			// CalendarID intentionally empty — syncer resolves the host's
			// default writable calendar.
		})
	}
	return targets
}

// recordConferencingFailure marks a booking with a conferencing-link failure so
// the dashboard surfaces it (similar to calendar sync failures), and notifies
// the host. The booking itself is still confirmed — issue #38 explicitly asks
// us to "create a booking regardless".
func (s *BookingService) recordConferencingFailure(ctx context.Context, details *BookingWithDetails, cause error) {
	if details.Booking.Answers == nil {
		details.Booking.Answers = models.JSONMap{}
	}
	reason := "transient"
	if errors.Is(cause, ErrConferencingReauthRequired) {
		reason = "reauth_required"
	}
	details.Booking.Answers["_conference_error"] = map[string]any{
		"provider": string(details.Template.LocationType),
		"reason":   reason,
		"message":  cause.Error(),
		"at":       time.Now().UTC().Format(time.RFC3339),
	}

	// Load host (may not be populated on details) for the notification email.
	host := details.Host
	if host == nil {
		h, hErr := s.repos.Host.GetByID(ctx, details.Booking.HostID)
		if hErr == nil {
			host = h
		}
	}
	if host != nil {
		s.email.SendConferencingFailed(ctx, host, string(details.Template.LocationType), reason, cause.Error())
	}
}
