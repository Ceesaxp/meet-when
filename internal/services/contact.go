package services

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// ContactService handles contact operations
type ContactService struct {
	repos *repository.Repositories
}

// NewContactService creates a new contact service
func NewContactService(repos *repository.Repositories) *ContactService {
	return &ContactService{repos: repos}
}

// UpsertFromBooking creates or updates a contact from a confirmed booking.
// Errors are logged but do not propagate to avoid blocking the booking flow.
func (s *ContactService) UpsertFromBooking(ctx context.Context, details *BookingWithDetails) {
	if details.Tenant == nil || details.Booking == nil {
		return
	}

	booking := details.Booking
	now := models.Now()
	bookingTime := booking.StartTime

	var phone *string
	if booking.InviteePhone != "" {
		phone = &booking.InviteePhone
	}

	var timezone *string
	if booking.InviteeTimezone != "" {
		timezone = &booking.InviteeTimezone
	}

	contact := &models.Contact{
		ID:           uuid.New().String(),
		TenantID:     details.Tenant.ID,
		Name:         booking.InviteeName,
		Email:        booking.InviteeEmail,
		Phone:        phone,
		Timezone:     timezone,
		FirstMet:     &bookingTime,
		LastMet:      &bookingTime,
		MeetingCount: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repos.Contact.Upsert(ctx, contact); err != nil {
		log.Printf("[CONTACT] Error upserting contact for %s: %v", booking.InviteeEmail, err)
	}
}

// UpsertFromHostedEventAttendee creates or updates a contact from a hosted-
// event attendee. The caller is expected to invoke this only for newly added
// attendees on create / on update — not for retained attendees on edit, so
// meeting_count and last_met don't get re-incremented from a benign edit.
// Errors are logged but do not propagate, mirroring UpsertFromBooking.
func (s *ContactService) UpsertFromHostedEventAttendee(ctx context.Context, tenantID string, email, name string, eventStart models.SQLiteTime) {
	if tenantID == "" || email == "" {
		return
	}
	now := models.Now()
	contact := &models.Contact{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		Name:         name,
		Email:        email,
		FirstMet:     &eventStart,
		LastMet:      &eventStart,
		MeetingCount: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repos.Contact.Upsert(ctx, contact); err != nil {
		log.Printf("[CONTACT] Error upserting contact for %s: %v", email, err)
	}
}

// BackfillFromBookings creates/updates contacts from all confirmed bookings for a tenant.
func (s *ContactService) BackfillFromBookings(ctx context.Context, tenantID string) (int, error) {
	bookings, err := s.repos.Booking.ListConfirmedByTenant(ctx, tenantID)
	if err != nil {
		return 0, err
	}

	count := 0
	now := models.Now()
	for _, booking := range bookings {
		var phone *string
		if booking.InviteePhone != "" {
			phone = &booking.InviteePhone
		}

		var timezone *string
		if booking.InviteeTimezone != "" {
			timezone = &booking.InviteeTimezone
		}

		bookingTime := booking.StartTime

		contact := &models.Contact{
			ID:           uuid.New().String(),
			TenantID:     tenantID,
			Name:         booking.InviteeName,
			Email:        booking.InviteeEmail,
			Phone:        phone,
			Timezone:     timezone,
			FirstMet:     &bookingTime,
			LastMet:      &bookingTime,
			MeetingCount: 1,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		if err := s.repos.Contact.Upsert(ctx, contact); err != nil {
			log.Printf("[CONTACT] Error backfilling contact for %s: %v", booking.InviteeEmail, err)
			continue
		}
		count++
	}

	return count, nil
}

// ListContacts returns contacts for a tenant with optional search
func (s *ContactService) ListContacts(ctx context.Context, tenantID, search string, offset, limit int) ([]*models.Contact, error) {
	return s.repos.Contact.List(ctx, tenantID, search, offset, limit)
}

// GetByEmail returns a contact by email
func (s *ContactService) GetByEmail(ctx context.Context, tenantID, email string) (*models.Contact, error) {
	return s.repos.Contact.GetByEmail(ctx, tenantID, email)
}

// GetBookings returns bookings for a contact with template names
func (s *ContactService) GetBookings(ctx context.Context, tenantID, email string) ([]*models.ContactBookingView, error) {
	return s.repos.Contact.GetBookings(ctx, tenantID, email)
}

// HasContacts checks if a tenant has any contacts
func (s *ContactService) HasContacts(ctx context.Context, tenantID string) (bool, error) {
	contacts, err := s.repos.Contact.List(ctx, tenantID, "", 0, 1)
	if err != nil {
		return false, err
	}
	return len(contacts) > 0, nil
}

// EnsureBackfilled checks if contacts exist and backfills from bookings if not
func (s *ContactService) EnsureBackfilled(ctx context.Context, tenantID string) error {
	has, err := s.HasContacts(ctx, tenantID)
	if err != nil {
		return err
	}
	if !has {
		count, err := s.BackfillFromBookings(ctx, tenantID)
		if err != nil {
			return err
		}
		if count > 0 {
			log.Printf("[CONTACT] Backfilled %d contacts for tenant %s", count, tenantID)
		}
	}
	return nil
}
