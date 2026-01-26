package services

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/meet-when/meet-when/internal/models"
)

// CalendarSyncService handles background calendar synchronization
type CalendarSyncService struct {
	calendar *CalendarService
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewCalendarSyncService creates a new calendar sync service
func NewCalendarSyncService(calendar *CalendarService) *CalendarSyncService {
	return &CalendarSyncService{
		calendar: calendar,
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

// syncCalendar syncs a single calendar connection
func (s *CalendarSyncService) syncCalendar(ctx context.Context, cal *models.CalendarConnection) error {
	return s.calendar.SyncCalendar(ctx, cal)
}
