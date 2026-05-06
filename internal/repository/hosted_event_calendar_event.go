package repository

import (
	"context"
	"database/sql"

	"github.com/meet-when/meet-when/internal/models"
)

// HostedEventCalendarEventRepository tracks per-host calendar events for a
// hosted event (parallel to BookingCalendarEventRepository).
type HostedEventCalendarEventRepository struct {
	db     *sql.DB
	driver string
}

func (r *HostedEventCalendarEventRepository) Create(ctx context.Context, e *models.HostedEventCalendarEvent) error {
	query := q(r.driver, `
		INSERT INTO hosted_event_calendar_events (id, hosted_event_id, host_id, calendar_id, event_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`)
	_, err := r.db.ExecContext(ctx, query,
		e.ID, e.HostedEventID, e.HostID, e.CalendarID, e.EventID, e.CreatedAt)
	return err
}

func (r *HostedEventCalendarEventRepository) GetByHostedEventID(ctx context.Context, eventID string) ([]*models.HostedEventCalendarEvent, error) {
	query := q(r.driver, `
		SELECT id, hosted_event_id, host_id, calendar_id, event_id, created_at
		FROM hosted_event_calendar_events
		WHERE hosted_event_id = $1
	`)
	rows, err := r.db.QueryContext(ctx, query, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.HostedEventCalendarEvent
	for rows.Next() {
		e := &models.HostedEventCalendarEvent{}
		if err := rows.Scan(&e.ID, &e.HostedEventID, &e.HostID, &e.CalendarID, &e.EventID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *HostedEventCalendarEventRepository) DeleteByHostedEventID(ctx context.Context, eventID string) error {
	query := q(r.driver, `DELETE FROM hosted_event_calendar_events WHERE hosted_event_id = $1`)
	_, err := r.db.ExecContext(ctx, query, eventID)
	return err
}

// Update changes the event_id (and optionally calendar_id) of a tracking row.
// Used after a CalDAV-style replace or a fall-back create on update.
func (r *HostedEventCalendarEventRepository) Update(ctx context.Context, e *models.HostedEventCalendarEvent) error {
	query := q(r.driver, `UPDATE hosted_event_calendar_events SET event_id = $1, calendar_id = $2 WHERE id = $3`)
	_, err := r.db.ExecContext(ctx, query, e.EventID, e.CalendarID, e.ID)
	return err
}
