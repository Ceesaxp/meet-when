package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
)

// ProviderCalendarRepository handles persistence for the per-calendar rows that
// live underneath a CalendarConnection (one row per Google calendar / CalDAV
// collection).
type ProviderCalendarRepository struct {
	db     *sql.DB
	driver string
}

const providerCalendarSelectColumns = `
	id, connection_id, provider_calendar_id, name, color, is_primary, is_writable,
	poll_busy, last_synced_at, COALESCE(sync_status, 'unknown'), COALESCE(sync_error, ''),
	created_at, updated_at`

func scanProviderCalendar(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.ProviderCalendar, error) {
	pc := &models.ProviderCalendar{}
	err := scanner.Scan(
		&pc.ID, &pc.ConnectionID, &pc.ProviderCalendarID, &pc.Name, &pc.Color,
		&pc.IsPrimary, &pc.IsWritable, &pc.PollBusy,
		&pc.LastSyncedAt, &pc.SyncStatus, &pc.SyncError,
		&pc.CreatedAt, &pc.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return pc, nil
}

// Create inserts a single provider_calendars row.
func (r *ProviderCalendarRepository) Create(ctx context.Context, pc *models.ProviderCalendar) error {
	query := q(r.driver, `
		INSERT INTO provider_calendars (
			id, connection_id, provider_calendar_id, name, color, is_primary, is_writable,
			poll_busy, last_synced_at, sync_status, sync_error, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`)
	_, err := r.db.ExecContext(ctx, query,
		pc.ID, pc.ConnectionID, pc.ProviderCalendarID, pc.Name, pc.Color,
		pc.IsPrimary, pc.IsWritable, pc.PollBusy,
		pc.LastSyncedAt, pc.SyncStatus, pc.SyncError,
		pc.CreatedAt, pc.UpdatedAt,
	)
	return err
}

// GetByID returns a single provider calendar.
func (r *ProviderCalendarRepository) GetByID(ctx context.Context, id string) (*models.ProviderCalendar, error) {
	query := q(r.driver, `SELECT `+providerCalendarSelectColumns+` FROM provider_calendars WHERE id = $1`)
	row := r.db.QueryRowContext(ctx, query, id)
	pc, err := scanProviderCalendar(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return pc, err
}

// GetByConnectionID returns all calendars under a single connection, ordered with
// the primary calendar first and then by name for stability.
func (r *ProviderCalendarRepository) GetByConnectionID(ctx context.Context, connectionID string) ([]*models.ProviderCalendar, error) {
	query := q(r.driver, `
		SELECT `+providerCalendarSelectColumns+`
		FROM provider_calendars
		WHERE connection_id = $1
		ORDER BY is_primary DESC, name ASC
	`)
	rows, err := r.db.QueryContext(ctx, query, connectionID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Printf("Error closing rows: %v", cerr)
		}
	}()

	var out []*models.ProviderCalendar
	for rows.Next() {
		pc, err := scanProviderCalendar(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pc)
	}
	return out, rows.Err()
}

// GetByHostID returns every provider calendar belonging to the host (across all
// of the host's connections), ordered for deterministic display.
func (r *ProviderCalendarRepository) GetByHostID(ctx context.Context, hostID string) ([]*models.ProviderCalendar, error) {
	query := q(r.driver, `
		SELECT pc.id, pc.connection_id, pc.provider_calendar_id, pc.name, pc.color,
		       pc.is_primary, pc.is_writable, pc.poll_busy,
		       pc.last_synced_at, COALESCE(pc.sync_status, 'unknown'), COALESCE(pc.sync_error, ''),
		       pc.created_at, pc.updated_at
		FROM provider_calendars pc
		JOIN calendar_connections cc ON cc.id = pc.connection_id
		WHERE cc.host_id = $1
		ORDER BY cc.created_at ASC, pc.is_primary DESC, pc.name ASC
	`)
	rows, err := r.db.QueryContext(ctx, query, hostID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Printf("Error closing rows: %v", cerr)
		}
	}()

	var out []*models.ProviderCalendar
	for rows.Next() {
		pc, err := scanProviderCalendar(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pc)
	}
	return out, rows.Err()
}

// GetPolledByHostID returns only the host's calendars where poll_busy = TRUE.
// Used by availability calculations.
func (r *ProviderCalendarRepository) GetPolledByHostID(ctx context.Context, hostID string) ([]*models.ProviderCalendar, error) {
	query := q(r.driver, `
		SELECT pc.id, pc.connection_id, pc.provider_calendar_id, pc.name, pc.color,
		       pc.is_primary, pc.is_writable, pc.poll_busy,
		       pc.last_synced_at, COALESCE(pc.sync_status, 'unknown'), COALESCE(pc.sync_error, ''),
		       pc.created_at, pc.updated_at
		FROM provider_calendars pc
		JOIN calendar_connections cc ON cc.id = pc.connection_id
		WHERE cc.host_id = $1 AND pc.poll_busy = TRUE
		ORDER BY cc.created_at ASC, pc.is_primary DESC, pc.name ASC
	`)
	// SQLite stores booleans as 0/1; the literal TRUE in the query is interpreted
	// as 1 by SQLite as well, so a single query works on both drivers.
	rows, err := r.db.QueryContext(ctx, query, hostID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			log.Printf("Error closing rows: %v", cerr)
		}
	}()

	var out []*models.ProviderCalendar
	for rows.Next() {
		pc, err := scanProviderCalendar(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pc)
	}
	return out, rows.Err()
}

// UpsertFromProvider inserts or updates a provider_calendars row keyed by
// (connection_id, provider_calendar_id). Existing user choices for poll_busy
// and a non-empty user-set color are preserved; the supplied color is only
// applied when the row is new or its color was unset.
//
// Returns the resulting row.
func (r *ProviderCalendarRepository) UpsertFromProvider(
	ctx context.Context,
	connectionID, providerCalendarID, name, color string,
	isPrimary, isWritable bool,
) (*models.ProviderCalendar, error) {
	// Try to find existing row first.
	findQuery := q(r.driver, `
		SELECT `+providerCalendarSelectColumns+`
		FROM provider_calendars
		WHERE connection_id = $1 AND provider_calendar_id = $2
	`)
	row := r.db.QueryRowContext(ctx, findQuery, connectionID, providerCalendarID)
	existing, err := scanProviderCalendar(row)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	now := models.Now()
	if existing != nil {
		// Refresh metadata that comes from the provider, but preserve user
		// preferences (poll_busy, color if set).
		newColor := existing.Color
		if newColor == "" {
			newColor = color
		}
		updateQuery := q(r.driver, `
			UPDATE provider_calendars
			SET name = $1, color = $2, is_primary = $3, is_writable = $4, updated_at = $5
			WHERE id = $6
		`)
		_, err := r.db.ExecContext(ctx, updateQuery,
			name, newColor, isPrimary, isWritable, now, existing.ID,
		)
		if err != nil {
			return nil, err
		}
		existing.Name = name
		existing.Color = newColor
		existing.IsPrimary = isPrimary
		existing.IsWritable = isWritable
		existing.UpdatedAt = now
		return existing, nil
	}

	pc := &models.ProviderCalendar{
		ID:                 uuid.New().String(),
		ConnectionID:       connectionID,
		ProviderCalendarID: providerCalendarID,
		Name:               name,
		Color:              color,
		IsPrimary:          isPrimary,
		IsWritable:         isWritable,
		PollBusy:           true,
		SyncStatus:         models.CalendarSyncStatusUnknown,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := r.Create(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// DeleteMissing removes calendars for the connection that are NOT in the
// provided keep list. Used during list-sync to drop calendars that disappeared
// at the provider.
func (r *ProviderCalendarRepository) DeleteMissing(ctx context.Context, connectionID string, keepProviderCalendarIDs []string) error {
	if len(keepProviderCalendarIDs) == 0 {
		query := q(r.driver, `DELETE FROM provider_calendars WHERE connection_id = $1`)
		_, err := r.db.ExecContext(ctx, query, connectionID)
		return err
	}

	// Build placeholder list compatible with both drivers.
	placeholders := ""
	args := []interface{}{connectionID}
	for i, id := range keepProviderCalendarIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	query := q(r.driver, fmt.Sprintf(`
		DELETE FROM provider_calendars
		WHERE connection_id = $1 AND provider_calendar_id NOT IN (%s)
	`, placeholders))
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// UpdatePollBusy toggles whether a calendar is polled for busy times. The host
// id is checked for ownership so that a user cannot toggle another tenant's
// calendar by guessing its id.
func (r *ProviderCalendarRepository) UpdatePollBusy(ctx context.Context, hostID, providerCalendarID string, pollBusy bool) error {
	query := q(r.driver, `
		UPDATE provider_calendars
		SET poll_busy = $1, updated_at = $2
		WHERE id = $3
		  AND connection_id IN (SELECT id FROM calendar_connections WHERE host_id = $4)
	`)
	res, err := r.db.ExecContext(ctx, query, pollBusy, models.Now(), providerCalendarID, hostID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("provider calendar %s not found or not owned by host %s", providerCalendarID, hostID)
	}
	return nil
}

// UpdateColor sets the user-chosen color for a provider calendar. Ownership
// check is identical to UpdatePollBusy.
func (r *ProviderCalendarRepository) UpdateColor(ctx context.Context, hostID, providerCalendarID, color string) error {
	query := q(r.driver, `
		UPDATE provider_calendars
		SET color = $1, updated_at = $2
		WHERE id = $3
		  AND connection_id IN (SELECT id FROM calendar_connections WHERE host_id = $4)
	`)
	res, err := r.db.ExecContext(ctx, query, color, models.Now(), providerCalendarID, hostID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("provider calendar %s not found or not owned by host %s", providerCalendarID, hostID)
	}
	return nil
}

// UpdateSyncStatus records the result of a sync attempt for a single calendar.
func (r *ProviderCalendarRepository) UpdateSyncStatus(ctx context.Context, providerCalendarID string, status models.CalendarSyncStatus, syncError string, lastSyncedAt *models.SQLiteTime) error {
	query := q(r.driver, `
		UPDATE provider_calendars
		SET sync_status = $1, sync_error = $2, last_synced_at = $3, updated_at = $4
		WHERE id = $5
	`)
	_, err := r.db.ExecContext(ctx, query, status, syncError, lastSyncedAt, models.Now(), providerCalendarID)
	return err
}

// Delete removes a provider calendar by id. ON DELETE SET NULL on
// meeting_templates and ON DELETE CASCADE on booking_calendar_events handle
// referential cleanup.
func (r *ProviderCalendarRepository) Delete(ctx context.Context, id string) error {
	query := q(r.driver, `DELETE FROM provider_calendars WHERE id = $1`)
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
