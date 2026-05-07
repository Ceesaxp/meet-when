package services

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/meet-when/meet-when/internal/repository"
)

// ReminderService handles sending booking reminder emails
type ReminderService struct {
	repos    *repository.Repositories
	email    *EmailService
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewReminderService creates a new reminder service
func NewReminderService(repos *repository.Repositories, email *EmailService) *ReminderService {
	return &ReminderService{
		repos:    repos,
		email:    email,
		interval: 15 * time.Minute, // Check every 15 minutes
		stopCh:   make(chan struct{}),
	}
}

// Start begins the background reminder checking loop
func (s *ReminderService) Start() {
	s.wg.Add(1)
	go s.run()
	log.Printf("[REMINDER] Service started, checking every %v", s.interval)
}

// Stop stops the background reminder checking loop
func (s *ReminderService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	log.Printf("[REMINDER] Service stopped")
}

func (s *ReminderService) run() {
	defer s.wg.Done()

	// Run immediately on startup
	s.checkAndSendReminders()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAndSendReminders()
		case <-s.stopCh:
			return
		}
	}
}

// checkAndSendReminders finds bookings and hosted events that need reminders
// and sends them. Booking and hosted-event sources are processed in separate
// sub-funcs so a partial failure in one source does not abort the other.
func (s *ReminderService) checkAndSendReminders() {
	ctx := context.Background()

	// Window: 23-25 hours from now (1-hour cushion either side of "tomorrow"
	// so we don't miss any items between ticks).
	now := time.Now().UTC()
	windowStart := now.Add(23 * time.Hour)
	windowEnd := now.Add(25 * time.Hour)

	s.processBookingReminders(ctx, windowStart, windowEnd)
	s.processHostedEventReminders(ctx, windowStart, windowEnd)
}

func (s *ReminderService) processBookingReminders(ctx context.Context, windowStart, windowEnd time.Time) {
	bookings, err := s.repos.Booking.GetBookingsNeedingReminder(ctx, windowStart, windowEnd)
	if err != nil {
		log.Printf("[REMINDER] Error fetching bookings needing reminders: %v", err)
		return
	}
	if len(bookings) == 0 {
		return
	}

	log.Printf("[REMINDER] Found %d bookings needing reminders", len(bookings))

	for _, booking := range bookings {
		template, err := s.repos.Template.GetByID(ctx, booking.TemplateID)
		if err != nil || template == nil {
			log.Printf("[REMINDER] Error fetching template %s: %v", booking.TemplateID, err)
			continue
		}

		host, err := s.repos.Host.GetByID(ctx, booking.HostID)
		if err != nil || host == nil {
			log.Printf("[REMINDER] Error fetching host %s: %v", booking.HostID, err)
			continue
		}

		tenant, err := s.repos.Tenant.GetByID(ctx, host.TenantID)
		if err != nil || tenant == nil {
			log.Printf("[REMINDER] Error fetching tenant %s: %v", host.TenantID, err)
			continue
		}

		details := &BookingWithDetails{
			Booking:  booking,
			Template: template,
			Host:     host,
			Tenant:   tenant,
		}

		s.email.SendBookingReminder(ctx, details)

		if err := s.repos.Booking.MarkReminderSent(ctx, booking.ID); err != nil {
			log.Printf("[REMINDER] Error marking reminder sent for booking %s: %v", booking.ID, err)
		} else {
			log.Printf("[REMINDER] Sent reminder for booking %s (invitee: %s, meeting: %s at %s)",
				booking.ID, booking.InviteeEmail, template.Name, booking.StartTime.Format(time.RFC3339))
		}
	}
}

func (s *ReminderService) processHostedEventReminders(ctx context.Context, windowStart, windowEnd time.Time) {
	events, err := s.repos.HostedEvent.GetUpcomingForReminders(ctx, windowStart, windowEnd)
	if err != nil {
		log.Printf("[REMINDER] Error fetching hosted events needing reminders: %v", err)
		return
	}
	if len(events) == 0 {
		return
	}

	log.Printf("[REMINDER] Found %d hosted events needing reminders", len(events))

	for _, event := range events {
		host, err := s.repos.Host.GetByID(ctx, event.HostID)
		if err != nil || host == nil {
			log.Printf("[REMINDER] Error fetching host %s for hosted event %s: %v", event.HostID, event.ID, err)
			continue
		}
		tenant, err := s.repos.Tenant.GetByID(ctx, event.TenantID)
		if err != nil || tenant == nil {
			log.Printf("[REMINDER] Error fetching tenant %s for hosted event %s: %v", event.TenantID, event.ID, err)
			continue
		}
		attendees, err := s.repos.HostedEventAttendee.ListByEvent(ctx, event.ID)
		if err != nil {
			log.Printf("[REMINDER] Error loading attendees for hosted event %s: %v", event.ID, err)
			continue
		}

		for _, a := range attendees {
			s.email.SendHostedEventReminder(ctx, event, a, host, tenant)
		}

		if err := s.repos.HostedEvent.MarkReminderSent(ctx, event.ID); err != nil {
			log.Printf("[REMINDER] Error marking reminder sent for hosted event %s: %v", event.ID, err)
		} else {
			log.Printf("[REMINDER] Sent reminder for hosted event %s (%d attendees, at %s)",
				event.ID, len(attendees), event.StartTime.Format(time.RFC3339))
		}
	}
}
