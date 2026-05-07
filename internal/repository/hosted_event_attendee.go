package repository

import (
	"context"
	"database/sql"
	"log"

	"github.com/meet-when/meet-when/internal/models"
)

// HostedEventAttendeeRepository handles hosted_event_attendees database
// operations.
type HostedEventAttendeeRepository struct {
	db     *sql.DB
	driver string
}

// ListByEvent returns all attendees for a hosted event, ordered by created_at.
func (r *HostedEventAttendeeRepository) ListByEvent(ctx context.Context, eventID string) ([]*models.HostedEventAttendee, error) {
	query := q(r.driver, `
		SELECT id, hosted_event_id, email, COALESCE(name, ''), contact_id, created_at
		FROM hosted_event_attendees
		WHERE hosted_event_id = $1
		ORDER BY created_at ASC
	`)
	rows, err := r.db.QueryContext(ctx, query, eventID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var out []*models.HostedEventAttendee
	for rows.Next() {
		a := &models.HostedEventAttendee{}
		if err := rows.Scan(&a.ID, &a.HostedEventID, &a.Email, &a.Name, &a.ContactID, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// ReplaceForEvent atomically replaces the attendee list for an event:
// deletes existing rows and inserts the new ones in a single transaction.
// Simpler than diffing and matches the "host re-submits the form" UX.
func (r *HostedEventAttendeeRepository) ReplaceForEvent(ctx context.Context, eventID string, attendees []*models.HostedEventAttendee) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, q(r.driver, `DELETE FROM hosted_event_attendees WHERE hosted_event_id = $1`), eventID); err != nil {
		return err
	}

	insertQuery := q(r.driver, `
		INSERT INTO hosted_event_attendees (id, hosted_event_id, email, name, contact_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`)
	for _, a := range attendees {
		a.HostedEventID = eventID
		if _, err := tx.ExecContext(ctx, insertQuery,
			a.ID, a.HostedEventID, a.Email, a.Name, a.ContactID, a.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
