package repository

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/meet-when/meet-when/internal/models"
)

// HostedEventRepository handles hosted_events database operations.
type HostedEventRepository struct {
	db     *sql.DB
	driver string
}

func (r *HostedEventRepository) Create(ctx context.Context, e *models.HostedEvent) error {
	query := q(r.driver, `
		INSERT INTO hosted_events (
			id, tenant_id, host_id, template_id, title, description,
			start_time, end_time, duration, timezone, location_type, custom_location,
			calendar_id, conference_link, status, cancel_reason, reminder_sent, is_archived,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`)
	_, err := r.db.ExecContext(ctx, query,
		e.ID, e.TenantID, e.HostID, e.TemplateID, e.Title, e.Description,
		e.StartTime, e.EndTime, e.Duration, e.Timezone, e.LocationType, e.CustomLocation,
		e.CalendarID, e.ConferenceLink, e.Status, e.CancelReason, e.ReminderSent, e.IsArchived,
		e.CreatedAt, e.UpdatedAt)
	return err
}

const hostedEventSelect = `
	SELECT id, tenant_id, host_id, template_id, title, description,
	       start_time, end_time, duration, timezone, location_type, custom_location,
	       calendar_id, conference_link, status, cancel_reason,
	       COALESCE(reminder_sent, false), COALESCE(is_archived, false),
	       created_at, updated_at
	FROM hosted_events
`

func scanHostedEvent(row interface {
	Scan(...interface{}) error
}) (*models.HostedEvent, error) {
	e := &models.HostedEvent{}
	err := row.Scan(
		&e.ID, &e.TenantID, &e.HostID, &e.TemplateID, &e.Title, &e.Description,
		&e.StartTime, &e.EndTime, &e.Duration, &e.Timezone, &e.LocationType, &e.CustomLocation,
		&e.CalendarID, &e.ConferenceLink, &e.Status, &e.CancelReason,
		&e.ReminderSent, &e.IsArchived,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (r *HostedEventRepository) GetByID(ctx context.Context, id string) (*models.HostedEvent, error) {
	query := q(r.driver, hostedEventSelect+` WHERE id = $1`)
	e, err := scanHostedEvent(r.db.QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

func (r *HostedEventRepository) Update(ctx context.Context, e *models.HostedEvent) error {
	query := q(r.driver, `
		UPDATE hosted_events SET
			title = $1, description = $2,
			start_time = $3, end_time = $4, duration = $5, timezone = $6,
			location_type = $7, custom_location = $8,
			calendar_id = $9, conference_link = $10,
			status = $11, cancel_reason = $12,
			reminder_sent = $13, is_archived = $14,
			updated_at = $15
		WHERE id = $16
	`)
	_, err := r.db.ExecContext(ctx, query,
		e.Title, e.Description,
		e.StartTime, e.EndTime, e.Duration, e.Timezone,
		e.LocationType, e.CustomLocation,
		e.CalendarID, e.ConferenceLink,
		e.Status, e.CancelReason,
		e.ReminderSent, e.IsArchived,
		e.UpdatedAt, e.ID)
	return err
}

func (r *HostedEventRepository) ListByHost(ctx context.Context, hostID string, includeArchived bool) ([]*models.HostedEvent, error) {
	query := hostedEventSelect + ` WHERE host_id = $1`
	if !includeArchived {
		query += ` AND (is_archived = false OR is_archived IS NULL)`
	}
	query += ` ORDER BY start_time DESC`
	rows, err := r.db.QueryContext(ctx, q(r.driver, query), hostID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var out []*models.HostedEvent
	for rows.Next() {
		e, err := scanHostedEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// GetByHostIDAndTimeRange returns non-cancelled hosted events for a host that
// overlap the given window. excludeEventID, when non-nil, skips that event —
// used by the conflict-detection endpoint when editing an existing event so
// it doesn't report itself as a conflict.
func (r *HostedEventRepository) GetByHostIDAndTimeRange(ctx context.Context, hostID string, start, end time.Time, excludeEventID *string) ([]*models.HostedEvent, error) {
	// Placeholders must appear in strict $1,$2,...,$N order: q(driver, ...)
	// replaces every "$N" with "?" for sqlite via plain regex substitution
	// (see repository.go's q()), so the positional bind order falls out of
	// the order placeholders appear in the query — not their numeric value.
	query := hostedEventSelect + ` WHERE host_id = $1 AND status = 'scheduled' AND end_time > $2 AND start_time < $3`
	// Format times to match the storage layout used by SQLiteTime.Value() so
	// the TEXT comparison under sqlite is reliable. Postgres tolerates the
	// same RFC-3339 string against TIMESTAMP columns.
	args := []interface{}{hostID, formatTimeArg(start), formatTimeArg(end)}
	if excludeEventID != nil {
		query += ` AND id <> $4`
		args = append(args, *excludeEventID)
	}
	query += ` ORDER BY start_time ASC`
	rows, err := r.db.QueryContext(ctx, q(r.driver, query), args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var out []*models.HostedEvent
	for rows.Next() {
		e, err := scanHostedEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// GetUpcomingForReminders returns scheduled hosted events with start_time in
// [start, end] that haven't had a reminder sent yet.
func (r *HostedEventRepository) GetUpcomingForReminders(ctx context.Context, start, end time.Time) ([]*models.HostedEvent, error) {
	query := q(r.driver, hostedEventSelect+`
		WHERE status = 'scheduled'
		  AND (reminder_sent = false OR reminder_sent IS NULL)
		  AND start_time >= $1
		  AND start_time <= $2
		ORDER BY start_time ASC
	`)
	rows, err := r.db.QueryContext(ctx, query, formatTimeArg(start), formatTimeArg(end))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var out []*models.HostedEvent
	for rows.Next() {
		e, err := scanHostedEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *HostedEventRepository) MarkReminderSent(ctx context.Context, id string) error {
	query := q(r.driver, `UPDATE hosted_events SET reminder_sent = true, updated_at = $1 WHERE id = $2`)
	_, err := r.db.ExecContext(ctx, query, models.Now(), id)
	return err
}

// formatTimeArg formats a time.Time into the RFC-3339 layout that
// SQLiteTime.Value() emits, so the same string comparison works under sqlite's
// TEXT columns regardless of how the driver would default-format raw
// time.Time values.
func formatTimeArg(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}
