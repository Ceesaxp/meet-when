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
	calendar     *CalendarService
	email        *EmailService
	repos        *repository.Repositories
	interval     time.Duration
	relistEvery  time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
	lastRelisted map[string]time.Time // connection ID -> last refreshed-list time
	mu           sync.Mutex
}

// NewCalendarSyncService creates a new calendar sync service
func NewCalendarSyncService(calendar *CalendarService, email *EmailService, repos *repository.Repositories) *CalendarSyncService {
	return &CalendarSyncService{
		calendar:     calendar,
		email:        email,
		repos:        repos,
		interval:     15 * time.Minute, // Busy-times sync every 15 minutes
		relistEvery:  60 * time.Minute, // Re-enumerate provider calendar lists hourly
		stopCh:       make(chan struct{}),
		lastRelisted: make(map[string]time.Time),
	}
}

// Start begins the background calendar sync loop
func (s *CalendarSyncService) Start() {
	s.wg.Add(1)
	go s.run()
	log.Printf("[CALENDAR_SYNC] Service started, syncing every %v (relist every %v)", s.interval, s.relistEvery)
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

	connections, err := s.calendar.GetAllCalendars(ctx)
	if err != nil {
		log.Printf("[CALENDAR_SYNC] Error fetching calendars: %v", err)
		return
	}

	if len(connections) == 0 {
		return
	}

	log.Printf("[CALENDAR_SYNC] Syncing %d connection(s)", len(connections))

	successCount := 0
	failCount := 0

	for _, conn := range connections {
		// Periodically re-list calendars to pick up additions/removals at the
		// provider. The list-fetch is rate-limited per connection.
		if s.shouldRelist(conn.ID) {
			if _, err := s.calendar.RefreshConnectionCalendarList(ctx, conn.HostID, conn.ID); err != nil {
				log.Printf("[CALENDAR_SYNC] re-list failed for connection %s: %v", conn.ID, err)
			} else {
				s.markRelisted(conn.ID)
			}
		}

		err := s.syncCalendar(ctx, conn)
		if err != nil {
			failCount++
			log.Printf("[CALENDAR_SYNC] Failed to sync connection %s (%s): %v", conn.ID, conn.Name, err)
		} else {
			successCount++
		}
	}

	log.Printf("[CALENDAR_SYNC] Sync complete: %d succeeded, %d failed", successCount, failCount)
}

// syncCalendar syncs a single calendar connection (i.e. all its
// provider_calendars) and detects failure transitions.
func (s *CalendarSyncService) syncCalendar(ctx context.Context, conn *models.CalendarConnection) error {
	previousStatus := conn.SyncStatus

	err := s.calendar.RefreshCalendarSync(ctx, conn.HostID, conn.ID)

	// Detect transition to failed state: send one-time email notification
	if err != nil && previousStatus != models.CalendarSyncStatusFailed {
		s.notifySyncFailure(ctx, conn, err)
	}

	return err
}

// shouldRelist reports whether this connection's calendar list is due for
// re-enumeration.
func (s *CalendarSyncService) shouldRelist(connectionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	last, ok := s.lastRelisted[connectionID]
	if !ok {
		return true
	}
	return time.Since(last) >= s.relistEvery
}

func (s *CalendarSyncService) markRelisted(connectionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRelisted[connectionID] = time.Now()
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
