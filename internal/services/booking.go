package services

import (
	"context"
	"errors"
	"log"
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

// BookingService handles booking operations
type BookingService struct {
	cfg          *config.Config
	repos        *repository.Repositories
	calendar     *CalendarService
	conferencing *ConferencingService
	email        *EmailService
	auditLog     *AuditLogService
}

// NewBookingService creates a new booking service
func NewBookingService(
	cfg *config.Config,
	repos *repository.Repositories,
	calendar *CalendarService,
	conferencing *ConferencingService,
	email *EmailService,
	auditLog *AuditLogService,
) *BookingService {
	return &BookingService{
		cfg:          cfg,
		repos:        repos,
		calendar:     calendar,
		conferencing: conferencing,
		email:        email,
		auditLog:     auditLog,
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

	// Calculate end time
	endTime := input.StartTime.Add(time.Duration(input.Duration) * time.Minute)

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

	// Get host and tenant for emails
	host, _ := s.repos.Host.GetByID(ctx, input.HostID)
	tenant, _ := s.repos.Tenant.GetByID(ctx, input.TenantID)

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

	// Delete calendar event if exists
	if booking.CalendarEventID != "" {
		template, _ := s.repos.Template.GetByID(ctx, booking.TemplateID)
		if template != nil {
			_ = s.calendar.DeleteEvent(ctx, booking.HostID, template.CalendarID, booking.CalendarEventID)
		}
	}

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
func (s *BookingService) GetBookings(ctx context.Context, hostID string, status *models.BookingStatus) ([]*models.Booking, error) {
	return s.repos.Booking.GetByHostID(ctx, hostID, status)
}

// GetPendingBookings retrieves pending bookings for a host
func (s *BookingService) GetPendingBookings(ctx context.Context, hostID string) ([]*models.Booking, error) {
	status := models.BookingStatusPending
	return s.repos.Booking.GetByHostID(ctx, hostID, &status)
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

	// Calculate new end time
	newEndTime := input.NewStartTime.Add(time.Duration(input.NewDuration) * time.Minute)

	// Validate time is in the future with minimum notice
	minNotice := time.Duration(template.MinNoticeMinutes) * time.Minute
	if input.NewStartTime.Before(time.Now().Add(minNotice)) {
		return nil, time.Time{}, ErrInvalidBookingTime
	}

	// Delete old calendar event if it exists
	if oldBooking.CalendarEventID != "" {
		_ = s.calendar.DeleteEvent(ctx, oldBooking.HostID, template.CalendarID, oldBooking.CalendarEventID)
	}

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

	// Get host and tenant
	host, _ := s.repos.Host.GetByID(ctx, oldBooking.HostID)
	tenant, _ := s.repos.Tenant.GetByID(ctx, host.TenantID)

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

		// Create new calendar event
		eventID, err := s.calendar.CreateEvent(ctx, details)
		if err != nil {
			log.Printf("[RESCHEDULE] Error creating calendar event: %v", err)
		} else if eventID != "" {
			details.Booking.CalendarEventID = eventID
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

// processConfirmedBooking handles post-confirmation actions
func (s *BookingService) processConfirmedBooking(ctx context.Context, details *BookingWithDetails) error {
	log.Printf("[BOOKING] processConfirmedBooking: booking=%s template=%s calendar=%s",
		details.Booking.ID, details.Template.ID, details.Template.CalendarID)

	// Create conference link if needed
	if details.Template.LocationType == models.ConferencingProviderGoogleMeet ||
		details.Template.LocationType == models.ConferencingProviderZoom {
		log.Printf("[BOOKING] Creating conference link for location type: %s", details.Template.LocationType)
		link, err := s.conferencing.CreateMeeting(ctx, details)
		if err != nil {
			log.Printf("[BOOKING] Error creating conference link: %v", err)
		} else if link != "" {
			log.Printf("[BOOKING] Conference link created: %s", link)
			details.Booking.ConferenceLink = link
		}
	}

	// Create calendar event
	log.Printf("[BOOKING] Creating calendar event...")
	eventID, err := s.calendar.CreateEvent(ctx, details)
	if err != nil {
		log.Printf("[BOOKING] Error creating calendar event: %v", err)
	} else if eventID != "" {
		log.Printf("[BOOKING] Calendar event created: %s", eventID)
		details.Booking.CalendarEventID = eventID
	} else {
		log.Printf("[BOOKING] No calendar event created (no calendar configured or empty response)")
	}

	// Update booking with conference link and event ID
	if err := s.repos.Booking.Update(ctx, details.Booking); err != nil {
		log.Printf("[BOOKING] Error updating booking: %v", err)
		return err
	}

	// Send confirmation emails
	s.email.SendBookingConfirmed(ctx, details)

	return nil
}
