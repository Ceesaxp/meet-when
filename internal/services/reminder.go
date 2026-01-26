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

// checkAndSendReminders finds bookings that need reminders and sends them
func (s *ReminderService) checkAndSendReminders() {
	ctx := context.Background()

	// Find confirmed bookings starting in the next 24-25 hours that haven't had reminders sent
	// We use a 1-hour window (24-25 hours) to ensure we don't miss any bookings
	// and don't send reminders too early
	now := time.Now().UTC()
	reminderWindowStart := now.Add(23 * time.Hour) // Start checking 23 hours from now
	reminderWindowEnd := now.Add(25 * time.Hour)   // Up to 25 hours from now

	bookings, err := s.repos.Booking.GetBookingsNeedingReminder(ctx, reminderWindowStart, reminderWindowEnd)
	if err != nil {
		log.Printf("[REMINDER] Error fetching bookings needing reminders: %v", err)
		return
	}

	if len(bookings) == 0 {
		return
	}

	log.Printf("[REMINDER] Found %d bookings needing reminders", len(bookings))

	for _, booking := range bookings {
		// Get related entities
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

		// Send the reminder email
		s.email.SendBookingReminder(ctx, details)

		// Mark the reminder as sent
		if err := s.repos.Booking.MarkReminderSent(ctx, booking.ID); err != nil {
			log.Printf("[REMINDER] Error marking reminder sent for booking %s: %v", booking.ID, err)
		} else {
			log.Printf("[REMINDER] Sent reminder for booking %s (invitee: %s, meeting: %s at %s)",
				booking.ID, booking.InviteeEmail, template.Name, booking.StartTime.Format(time.RFC3339))
		}
	}
}
