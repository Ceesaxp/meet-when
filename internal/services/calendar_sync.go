package services

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// CalendarSyncService handles background calendar synchronization
type CalendarSyncService struct {
	calendar *CalendarService
	email    *EmailService
	repos    *repository.Repositories
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewCalendarSyncService creates a new calendar sync service
func NewCalendarSyncService(calendar *CalendarService, email *EmailService, repos *repository.Repositories) *CalendarSyncService {
	return &CalendarSyncService{
		calendar: calendar,
		email:    email,
		repos:    repos,
		interval: 15 * time.Minute, // Sync every 15 minutes
		stopCh:   make(chan struct{}),
	}
}

// Start begins the background calendar sync loop
func (s *CalendarSyncService) Start() {
	s.wg.Add(1)
	go s.run()
	log.Printf("[CALENDAR_SYNC] Service started, syncing every %v", s.interval)
}

// Stop stops the background calendar sync loop
func (s *CalendarSyncService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	log.Printf("[CALENDAR_SYNC] Service stopped")
}

func (s *CalendarSyncService) run() {
	defer s.wg.Done()

	// Run immediately on startup
	s.syncAllCalendars()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.syncAllCalendars()
		case <-s.stopCh:
			return
		}
	}
}

// syncAllCalendars fetches all calendar connections and syncs each one
func (s *CalendarSyncService) syncAllCalendars() {
	ctx := context.Background()

	calendars, err := s.calendar.GetAllCalendars(ctx)
	if err != nil {
		log.Printf("[CALENDAR_SYNC] Error fetching calendars: %v", err)
		return
	}

	if len(calendars) == 0 {
		return
	}

	log.Printf("[CALENDAR_SYNC] Syncing %d calendar(s)", len(calendars))

	successCount := 0
	failCount := 0

	for _, cal := range calendars {
		err := s.syncCalendar(ctx, cal)
		if err != nil {
			failCount++
			log.Printf("[CALENDAR_SYNC] Failed to sync calendar %s (%s): %v", cal.ID, cal.Name, err)
		} else {
			successCount++
		}
	}

	log.Printf("[CALENDAR_SYNC] Sync complete: %d succeeded, %d failed", successCount, failCount)
}

// syncCalendar syncs a single calendar connection and detects failure transitions
func (s *CalendarSyncService) syncCalendar(ctx context.Context, cal *models.CalendarConnection) error {
	previousStatus := cal.SyncStatus

	err := s.calendar.SyncCalendar(ctx, cal)

	// Detect transition to failed state: send one-time email notification
	if err != nil && previousStatus != models.CalendarSyncStatusFailed {
		s.notifySyncFailure(ctx, cal, err)
	}

	return err
}

// notifySyncFailure sends a one-time email to the host when a calendar transitions to failed
func (s *CalendarSyncService) notifySyncFailure(ctx context.Context, cal *models.CalendarConnection, syncErr error) {
	host, err := s.repos.Host.GetByID(ctx, cal.HostID)
	if err != nil || host == nil {
		log.Printf("[CALENDAR_SYNC] Could not load host %s for sync failure notification: %v", cal.HostID, err)
		return
	}

	errMsg := syncErr.Error()
	if errors.Is(syncErr, ErrCalendarAuth) {
		errMsg = "Authentication expired. Please reconnect your calendar."
	}

	log.Printf("[CALENDAR_SYNC] Calendar %s (%s) transitioned to failed for host %s, sending notification", cal.ID, cal.Name, host.Email)
	s.email.SendCalendarSyncFailed(ctx, host, cal.Name, errMsg)
}
