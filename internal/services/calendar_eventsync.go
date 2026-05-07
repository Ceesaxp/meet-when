package services

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// ScheduledItemKind identifies what kind of scheduled item a CalendarSyncRequest
// belongs to. The syncer routes tracking-table writes off this value.
type ScheduledItemKind string

const (
	ItemKindBooking     ScheduledItemKind = "booking"
	ItemKindHostedEvent ScheduledItemKind = "hosted_event"
)

// HostTarget identifies one host's calendar that an event should be written to.
type HostTarget struct {
	HostID     string
	CalendarID string // empty → resolve via host default + first writable
	IsOwner    bool
}

// CalendarSyncRequest is the unified shape that BookingService and
// HostedEventService hand to the syncer.
type CalendarSyncRequest struct {
	Kind   ScheduledItemKind
	ItemID string
	Input  CalendarEventInput
	Hosts  []HostTarget
}

// trackingRow is the syncer's view of a row in either booking_calendar_events
// or hosted_event_calendar_events. It's the minimum information the syncer
// needs to update or delete provider events.
type trackingRow struct {
	ID         string
	HostID     string
	CalendarID string
	EventID    string
}

// trackingStore abstracts the per-kind tracking table the syncer writes to.
// One implementation per ScheduledItemKind.
type trackingStore interface {
	create(ctx context.Context, itemID, hostID, calendarID, eventID string) error
	listByItemID(ctx context.Context, itemID string) ([]trackingRow, error)
	updateEventID(ctx context.Context, rowID, newEventID, newCalendarID string) error
	deleteByItemID(ctx context.Context, itemID string) error
}

// CalendarEventSyncer fans calendar create/update/delete out across one or
// more host calendars and persists tracking rows. Replaces the duplicated
// per-host logic that previously lived inside BookingService.
type CalendarEventSyncer struct {
	repos    *repository.Repositories
	calendar calendarEventWriter
	stores   map[ScheduledItemKind]trackingStore
}

// NewCalendarEventSyncer constructs the syncer with both kind dispatchers
// wired.
func NewCalendarEventSyncer(repos *repository.Repositories, calendar calendarEventWriter) *CalendarEventSyncer {
	return &CalendarEventSyncer{
		repos:    repos,
		calendar: calendar,
		stores: map[ScheduledItemKind]trackingStore{
			ItemKindBooking:     &bookingTrackingStore{repo: repos.BookingCalendarEvent},
			ItemKindHostedEvent: &hostedEventTrackingStore{repo: repos.HostedEventCalendarEvent},
		},
	}
}

// resolveWritableCalendarID returns the calendar ID to write to for a host:
// preferredID if non-empty, else the host's default, else the first writable
// provider calendar. Returns ("", error) only on lookup error; ("", nil) means
// no writable calendar exists for this host (caller should skip).
func (s *CalendarEventSyncer) resolveWritableCalendarID(ctx context.Context, hostID, preferredID string) (string, error) {
	if preferredID != "" {
		return preferredID, nil
	}
	host, err := s.repos.Host.GetByID(ctx, hostID)
	if err != nil {
		return "", err
	}
	if host != nil && host.DefaultCalendarID != nil && *host.DefaultCalendarID != "" {
		return *host.DefaultCalendarID, nil
	}
	pcs, err := s.repos.ProviderCalendar.GetByHostID(ctx, hostID)
	if err != nil {
		return "", err
	}
	for _, pc := range pcs {
		if pc.IsWritable {
			return pc.ID, nil
		}
	}
	return "", nil
}

// Create writes the calendar event for every host target and persists one
// tracking row per success. Returns the first non-empty event ID (used as the
// "primary" ID by callers that keep a legacy single-event-id column) and the
// first non-empty conference link surfaced by the create response (Google
// Meet path). Per-host errors are logged but do not abort the batch — matches
// the prior booking behaviour.
func (s *CalendarEventSyncer) Create(ctx context.Context, req CalendarSyncRequest) (string, string, error) {
	store, err := s.storeFor(req.Kind)
	if err != nil {
		return "", "", err
	}

	var firstEventID, firstConferenceLink string
	for _, target := range req.Hosts {
		calendarID, err := s.resolveWritableCalendarID(ctx, target.HostID, target.CalendarID)
		if err != nil {
			log.Printf("[SYNC] Error resolving calendar for host %s: %v", target.HostID, err)
			continue
		}
		if calendarID == "" {
			log.Printf("[SYNC] No writable calendar for host %s, skipping event creation", target.HostID)
			continue
		}

		// The provider may need the per-host EventID empty (always for create).
		input := req.Input
		input.EventID = ""

		eventID, conferenceLink, err := s.calendar.CreateEventForHost(ctx, calendarID, &input)
		if err != nil {
			log.Printf("[SYNC] Error creating calendar event for host %s: %v", target.HostID, err)
			continue
		}
		if eventID == "" {
			continue
		}

		if err := store.create(ctx, req.ItemID, target.HostID, calendarID, eventID); err != nil {
			log.Printf("[SYNC] Error writing tracking row for %s/%s: %v", req.Kind, req.ItemID, err)
		}

		if firstEventID == "" && target.IsOwner {
			firstEventID = eventID
			firstConferenceLink = conferenceLink
		} else if firstEventID == "" {
			firstEventID = eventID
			firstConferenceLink = conferenceLink
		}
	}
	return firstEventID, firstConferenceLink, nil
}

// Update patches every tracked calendar event for the item. If a provider's
// update returns a replacement event ID (CalDAV delete+create, or the
// fallback-to-create path when the prior ID was empty), the tracking row is
// updated in place — without this, subsequent updates / deletes would target
// stale events.
func (s *CalendarEventSyncer) Update(ctx context.Context, req CalendarSyncRequest) error {
	store, err := s.storeFor(req.Kind)
	if err != nil {
		return err
	}

	rows, err := store.listByItemID(ctx, req.ItemID)
	if err != nil {
		return fmt.Errorf("list tracking rows: %w", err)
	}

	for _, row := range rows {
		input := req.Input
		input.EventID = row.EventID

		newID, err := s.calendar.UpdateEvent(ctx, row.CalendarID, &input)
		if err != nil {
			log.Printf("[SYNC] Error updating calendar event %s for host %s: %v", row.EventID, row.HostID, err)
			continue
		}
		if newID != "" && newID != row.EventID {
			if err := store.updateEventID(ctx, row.ID, newID, row.CalendarID); err != nil {
				log.Printf("[SYNC] Error persisting replacement event ID for %s/%s: %v", req.Kind, req.ItemID, err)
			}
		}
	}
	return nil
}

// Delete removes the calendar event for every host target and clears the
// tracking rows. Returns the number of tracking rows that existed (callers
// can branch on 0 to fall back to legacy single-host paths).
func (s *CalendarEventSyncer) Delete(ctx context.Context, kind ScheduledItemKind, itemID string) (int, error) {
	store, err := s.storeFor(kind)
	if err != nil {
		return 0, err
	}

	rows, err := store.listByItemID(ctx, itemID)
	if err != nil {
		return 0, fmt.Errorf("list tracking rows: %w", err)
	}

	for _, row := range rows {
		if err := s.calendar.DeleteEvent(ctx, row.HostID, row.CalendarID, row.EventID); err != nil {
			log.Printf("[SYNC] Error deleting calendar event %s for host %s: %v", row.EventID, row.HostID, err)
		}
	}
	if len(rows) > 0 {
		if err := store.deleteByItemID(ctx, itemID); err != nil {
			log.Printf("[SYNC] Error clearing tracking rows for %s/%s: %v", kind, itemID, err)
		}
	}
	return len(rows), nil
}

func (s *CalendarEventSyncer) storeFor(kind ScheduledItemKind) (trackingStore, error) {
	store, ok := s.stores[kind]
	if !ok {
		return nil, errors.New("no tracking store registered for kind " + string(kind))
	}
	return store, nil
}

// bookingTrackingStore adapts BookingCalendarEventRepository to trackingStore.
type bookingTrackingStore struct {
	repo *repository.BookingCalendarEventRepository
}

func (b *bookingTrackingStore) create(ctx context.Context, itemID, hostID, calendarID, eventID string) error {
	return b.repo.Create(ctx, &models.BookingCalendarEvent{
		ID:         uuid.New().String(),
		BookingID:  itemID,
		HostID:     hostID,
		CalendarID: calendarID,
		EventID:    eventID,
		CreatedAt:  models.Now(),
	})
}

func (b *bookingTrackingStore) listByItemID(ctx context.Context, itemID string) ([]trackingRow, error) {
	events, err := b.repo.GetByBookingID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	out := make([]trackingRow, 0, len(events))
	for _, e := range events {
		out = append(out, trackingRow{
			ID:         e.ID,
			HostID:     e.HostID,
			CalendarID: e.CalendarID,
			EventID:    e.EventID,
		})
	}
	return out, nil
}

func (b *bookingTrackingStore) updateEventID(ctx context.Context, rowID, newEventID, newCalendarID string) error {
	return b.repo.Update(ctx, &models.BookingCalendarEvent{
		ID:         rowID,
		EventID:    newEventID,
		CalendarID: newCalendarID,
	})
}

func (b *bookingTrackingStore) deleteByItemID(ctx context.Context, itemID string) error {
	return b.repo.DeleteByBookingID(ctx, itemID)
}

// hostedEventTrackingStore adapts HostedEventCalendarEventRepository.
type hostedEventTrackingStore struct {
	repo *repository.HostedEventCalendarEventRepository
}

func (h *hostedEventTrackingStore) create(ctx context.Context, itemID, hostID, calendarID, eventID string) error {
	return h.repo.Create(ctx, &models.HostedEventCalendarEvent{
		ID:            uuid.New().String(),
		HostedEventID: itemID,
		HostID:        hostID,
		CalendarID:    calendarID,
		EventID:       eventID,
		CreatedAt:     models.Now(),
	})
}

func (h *hostedEventTrackingStore) listByItemID(ctx context.Context, itemID string) ([]trackingRow, error) {
	events, err := h.repo.GetByHostedEventID(ctx, itemID)
	if err != nil {
		return nil, err
	}
	out := make([]trackingRow, 0, len(events))
	for _, e := range events {
		out = append(out, trackingRow{
			ID:         e.ID,
			HostID:     e.HostID,
			CalendarID: e.CalendarID,
			EventID:    e.EventID,
		})
	}
	return out, nil
}

func (h *hostedEventTrackingStore) updateEventID(ctx context.Context, rowID, newEventID, newCalendarID string) error {
	return h.repo.Update(ctx, &models.HostedEventCalendarEvent{
		ID:         rowID,
		EventID:    newEventID,
		CalendarID: newCalendarID,
	})
}

func (h *hostedEventTrackingStore) deleteByItemID(ctx context.Context, itemID string) error {
	return h.repo.DeleteByHostedEventID(ctx, itemID)
}
