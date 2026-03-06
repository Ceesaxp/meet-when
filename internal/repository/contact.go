package repository

import (
	"context"
	"database/sql"
	"log"

	"github.com/meet-when/meet-when/internal/models"
)

// ContactRepository handles contact database operations
type ContactRepository struct {
	db     *sql.DB
	driver string
}

// Upsert creates or updates a contact atomically using ON CONFLICT
func (r *ContactRepository) Upsert(ctx context.Context, contact *models.Contact) error {
	var query string
	if r.driver == "sqlite" {
		query = `
			INSERT INTO contacts (id, tenant_id, name, email, phone, timezone, first_met, last_met, meeting_count, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (tenant_id, email) DO UPDATE SET
				name = CASE WHEN contacts.name = '' THEN excluded.name ELSE contacts.name END,
				phone = COALESCE(contacts.phone, excluded.phone),
				timezone = COALESCE(contacts.timezone, excluded.timezone),
				last_met = CASE WHEN excluded.last_met > COALESCE(contacts.last_met, '1970-01-01') THEN excluded.last_met ELSE contacts.last_met END,
				meeting_count = contacts.meeting_count + excluded.meeting_count,
				updated_at = excluded.updated_at
		`
	} else {
		query = `
			INSERT INTO contacts (id, tenant_id, name, email, phone, timezone, first_met, last_met, meeting_count, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (tenant_id, email) DO UPDATE SET
				name = CASE WHEN contacts.name = '' THEN EXCLUDED.name ELSE contacts.name END,
				phone = COALESCE(contacts.phone, EXCLUDED.phone),
				timezone = COALESCE(contacts.timezone, EXCLUDED.timezone),
				last_met = CASE WHEN EXCLUDED.last_met > COALESCE(contacts.last_met, '1970-01-01'::timestamp) THEN EXCLUDED.last_met ELSE contacts.last_met END,
				meeting_count = contacts.meeting_count + EXCLUDED.meeting_count,
				updated_at = EXCLUDED.updated_at
		`
	}

	_, err := r.db.ExecContext(ctx, query,
		contact.ID, contact.TenantID, contact.Name, contact.Email,
		contact.Phone, contact.Timezone, contact.FirstMet, contact.LastMet,
		contact.MeetingCount, contact.CreatedAt, contact.UpdatedAt)
	return err
}

// List returns contacts for a tenant with optional search filter, sorted by last_met descending
func (r *ContactRepository) List(ctx context.Context, tenantID, search string, offset, limit int) ([]*models.Contact, error) {
	var query string
	var args []any

	if search != "" {
		searchPattern := "%" + search + "%"
		if r.driver == "sqlite" {
			query = `
				SELECT id, tenant_id, name, email, phone, timezone, first_met, last_met,
				       meeting_count, created_at, updated_at
				FROM contacts
				WHERE tenant_id = ? AND (name LIKE ? OR email LIKE ?)
				ORDER BY last_met DESC NULLS LAST
				LIMIT ? OFFSET ?
			`
			args = []any{tenantID, searchPattern, searchPattern, limit, offset}
		} else {
			query = `
				SELECT id, tenant_id, name, email, phone, timezone, first_met, last_met,
				       meeting_count, created_at, updated_at
				FROM contacts
				WHERE tenant_id = $1 AND (name ILIKE $2 OR email ILIKE $3)
				ORDER BY last_met DESC NULLS LAST
				LIMIT $4 OFFSET $5
			`
			args = []any{tenantID, searchPattern, searchPattern, limit, offset}
		}
	} else {
		query = q(r.driver, `
			SELECT id, tenant_id, name, email, phone, timezone, first_met, last_met,
			       meeting_count, created_at, updated_at
			FROM contacts
			WHERE tenant_id = $1
			ORDER BY last_met DESC NULLS LAST
			LIMIT $2 OFFSET $3
		`)
		args = []any{tenantID, limit, offset}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var contacts []*models.Contact
	for rows.Next() {
		c := &models.Contact{}
		err := rows.Scan(
			&c.ID, &c.TenantID, &c.Name, &c.Email, &c.Phone, &c.Timezone,
			&c.FirstMet, &c.LastMet, &c.MeetingCount, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contacts, nil
}

// GetByEmail returns a single contact by tenant ID and email
func (r *ContactRepository) GetByEmail(ctx context.Context, tenantID, email string) (*models.Contact, error) {
	c := &models.Contact{}
	query := q(r.driver, `
		SELECT id, tenant_id, name, email, phone, timezone, first_met, last_met,
		       meeting_count, created_at, updated_at
		FROM contacts
		WHERE tenant_id = $1 AND email = $2
	`)
	err := r.db.QueryRowContext(ctx, query, tenantID, email).Scan(
		&c.ID, &c.TenantID, &c.Name, &c.Email, &c.Phone, &c.Timezone,
		&c.FirstMet, &c.LastMet, &c.MeetingCount, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// GetBookings returns bookings for a given contact email within a tenant
func (r *ContactRepository) GetBookings(ctx context.Context, tenantID, email string) ([]*models.Booking, error) {
	query := q(r.driver, `
		SELECT b.id, b.template_id, b.host_id, b.token, b.status, b.start_time, b.end_time, b.duration,
		       b.invitee_name, b.invitee_email, COALESCE(b.invitee_timezone, ''), COALESCE(b.invitee_phone, ''),
		       b.additional_guests, b.answers, COALESCE(b.conference_link, ''), COALESCE(b.calendar_event_id, ''),
		       COALESCE(b.cancelled_by, ''), COALESCE(b.cancel_reason, ''), COALESCE(b.reminder_sent, false),
		       COALESCE(b.is_archived, false), b.created_at, b.updated_at
		FROM bookings b
		JOIN hosts h ON b.host_id = h.id
		WHERE h.tenant_id = $1 AND b.invitee_email = $2
		ORDER BY b.start_time DESC
	`)

	rows, err := r.db.QueryContext(ctx, query, tenantID, email)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var bookings []*models.Booking
	for rows.Next() {
		booking := &models.Booking{}
		err := rows.Scan(
			&booking.ID, &booking.TemplateID, &booking.HostID, &booking.Token,
			&booking.Status, &booking.StartTime, &booking.EndTime, &booking.Duration,
			&booking.InviteeName, &booking.InviteeEmail, &booking.InviteeTimezone,
			&booking.InviteePhone, &booking.AdditionalGuests, &booking.Answers,
			&booking.ConferenceLink, &booking.CalendarEventID,
			&booking.CancelledBy, &booking.CancelReason, &booking.ReminderSent,
			&booking.IsArchived, &booking.CreatedAt, &booking.UpdatedAt)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, booking)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bookings, nil
}
