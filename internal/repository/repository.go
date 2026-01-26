package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/meet-when/meet-when/internal/models"
)

// Repositories holds all repository instances
type Repositories struct {
	Tenant       *TenantRepository
	Host         *HostRepository
	Calendar     *CalendarRepository
	Conferencing *ConferencingRepository
	Template     *TemplateRepository
	Booking      *BookingRepository
	Session      *SessionRepository
	WorkingHours *WorkingHoursRepository
	AuditLog     *AuditLogRepository
}

// NewRepositories creates all repositories
func NewRepositories(db *sql.DB, driver string) *Repositories {
	return &Repositories{
		Tenant:       &TenantRepository{db: db, driver: driver},
		Host:         &HostRepository{db: db, driver: driver},
		Calendar:     &CalendarRepository{db: db, driver: driver},
		Conferencing: &ConferencingRepository{db: db, driver: driver},
		Template:     &TemplateRepository{db: db, driver: driver},
		Booking:      &BookingRepository{db: db, driver: driver},
		Session:      &SessionRepository{db: db, driver: driver},
		WorkingHours: &WorkingHoursRepository{db: db, driver: driver},
		AuditLog:     &AuditLogRepository{db: db, driver: driver},
	}
}

// q converts PostgreSQL-style placeholders ($1, $2) to SQLite-style (?) if needed
func q(driver, query string) string {
	if driver == "sqlite" {
		re := regexp.MustCompile(`\$\d+`)
		return re.ReplaceAllString(query, "?")
	}
	return query
}

// TenantRepository handles tenant database operations
type TenantRepository struct {
	db     *sql.DB
	driver string
}

func (r *TenantRepository) Create(ctx context.Context, tenant *models.Tenant) error {
	query := q(r.driver, `
		INSERT INTO tenants (id, slug, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`)
	_, err := r.db.ExecContext(ctx, query,
		tenant.ID, tenant.Slug, tenant.Name, tenant.CreatedAt, tenant.UpdatedAt)
	return err
}

func (r *TenantRepository) GetByID(ctx context.Context, id string) (*models.Tenant, error) {
	tenant := &models.Tenant{}
	query := q(r.driver, `SELECT id, slug, name, created_at, updated_at FROM tenants WHERE id = $1`)
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&tenant.ID, &tenant.Slug, &tenant.Name, &tenant.CreatedAt, &tenant.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return tenant, err
}

func (r *TenantRepository) GetBySlug(ctx context.Context, slug string) (*models.Tenant, error) {
	tenant := &models.Tenant{}
	query := q(r.driver, `SELECT id, slug, name, created_at, updated_at FROM tenants WHERE slug = $1`)
	err := r.db.QueryRowContext(ctx, query, slug).Scan(
		&tenant.ID, &tenant.Slug, &tenant.Name, &tenant.CreatedAt, &tenant.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return tenant, err
}

// HostRepository handles host database operations
type HostRepository struct {
	db     *sql.DB
	driver string
}

func (r *HostRepository) Create(ctx context.Context, host *models.Host) error {
	query := q(r.driver, `
		INSERT INTO hosts (id, tenant_id, email, password_hash, name, slug, timezone, is_admin, onboarding_completed, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`)
	_, err := r.db.ExecContext(ctx, query,
		host.ID, host.TenantID, host.Email, host.PasswordHash, host.Name,
		host.Slug, host.Timezone, host.IsAdmin, host.OnboardingCompleted, host.CreatedAt, host.UpdatedAt)
	return err
}

func (r *HostRepository) GetByID(ctx context.Context, id string) (*models.Host, error) {
	host := &models.Host{}
	query := q(r.driver, `
		SELECT id, tenant_id, email, password_hash, name, slug, timezone,
		       default_calendar_id, is_admin, COALESCE(onboarding_completed, false), created_at, updated_at
		FROM hosts WHERE id = $1
	`)
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&host.ID, &host.TenantID, &host.Email, &host.PasswordHash, &host.Name,
		&host.Slug, &host.Timezone, &host.DefaultCalendarID, &host.IsAdmin,
		&host.OnboardingCompleted, &host.CreatedAt, &host.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return host, err
}

func (r *HostRepository) GetByEmail(ctx context.Context, tenantID, email string) (*models.Host, error) {
	host := &models.Host{}
	query := q(r.driver, `
		SELECT id, tenant_id, email, password_hash, name, slug, timezone,
		       default_calendar_id, is_admin, COALESCE(onboarding_completed, false), created_at, updated_at
		FROM hosts WHERE tenant_id = $1 AND email = $2
	`)
	err := r.db.QueryRowContext(ctx, query, tenantID, email).Scan(
		&host.ID, &host.TenantID, &host.Email, &host.PasswordHash, &host.Name,
		&host.Slug, &host.Timezone, &host.DefaultCalendarID, &host.IsAdmin,
		&host.OnboardingCompleted, &host.CreatedAt, &host.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return host, err
}

func (r *HostRepository) GetBySlug(ctx context.Context, tenantID, slug string) (*models.Host, error) {
	host := &models.Host{}
	query := q(r.driver, `
		SELECT id, tenant_id, email, password_hash, name, slug, timezone,
		       default_calendar_id, is_admin, COALESCE(onboarding_completed, false), created_at, updated_at
		FROM hosts WHERE tenant_id = $1 AND slug = $2
	`)
	err := r.db.QueryRowContext(ctx, query, tenantID, slug).Scan(
		&host.ID, &host.TenantID, &host.Email, &host.PasswordHash, &host.Name,
		&host.Slug, &host.Timezone, &host.DefaultCalendarID, &host.IsAdmin,
		&host.OnboardingCompleted, &host.CreatedAt, &host.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return host, err
}

func (r *HostRepository) Update(ctx context.Context, host *models.Host) error {
	query := q(r.driver, `
		UPDATE hosts
		SET name = $1, slug = $2, timezone = $3, default_calendar_id = $4
		WHERE id = $5
	`)
	_, err := r.db.ExecContext(ctx, query,
		host.Name, host.Slug, host.Timezone, host.DefaultCalendarID, host.ID)
	return err
}

func (r *HostRepository) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	query := q(r.driver, `UPDATE hosts SET password_hash = $1 WHERE id = $2`)
	_, err := r.db.ExecContext(ctx, query, passwordHash, id)
	return err
}

func (r *HostRepository) UpdateOnboardingCompleted(ctx context.Context, id string, completed bool) error {
	query := q(r.driver, `UPDATE hosts SET onboarding_completed = $1 WHERE id = $2`)
	_, err := r.db.ExecContext(ctx, query, completed, id)
	return err
}

// CalendarRepository handles calendar connection database operations
type CalendarRepository struct {
	db     *sql.DB
	driver string
}

func (r *CalendarRepository) Create(ctx context.Context, cal *models.CalendarConnection) error {
	query := q(r.driver, `
		INSERT INTO calendar_connections (id, host_id, provider, name, calendar_id,
			access_token, refresh_token, token_expiry, caldav_url, caldav_username,
			caldav_password, is_default, last_synced_at, sync_status, sync_error,
			created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`)
	_, err := r.db.ExecContext(ctx, query,
		cal.ID, cal.HostID, cal.Provider, cal.Name, cal.CalendarID,
		cal.AccessToken, cal.RefreshToken, cal.TokenExpiry,
		cal.CalDAVURL, cal.CalDAVUsername, cal.CalDAVPassword,
		cal.IsDefault, cal.LastSyncedAt, cal.SyncStatus, cal.SyncError,
		cal.CreatedAt, cal.UpdatedAt)
	return err
}

func (r *CalendarRepository) GetByID(ctx context.Context, id string) (*models.CalendarConnection, error) {
	cal := &models.CalendarConnection{}
	query := q(r.driver, `
		SELECT id, host_id, provider, name, calendar_id, access_token, refresh_token,
		       token_expiry, caldav_url, caldav_username, caldav_password, is_default,
		       last_synced_at, COALESCE(sync_status, 'unknown'), COALESCE(sync_error, ''),
		       created_at, updated_at
		FROM calendar_connections WHERE id = $1
	`)
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&cal.ID, &cal.HostID, &cal.Provider, &cal.Name, &cal.CalendarID,
		&cal.AccessToken, &cal.RefreshToken, &cal.TokenExpiry,
		&cal.CalDAVURL, &cal.CalDAVUsername, &cal.CalDAVPassword,
		&cal.IsDefault, &cal.LastSyncedAt, &cal.SyncStatus, &cal.SyncError,
		&cal.CreatedAt, &cal.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return cal, err
}

func (r *CalendarRepository) GetByHostID(ctx context.Context, hostID string) ([]*models.CalendarConnection, error) {
	query := q(r.driver, `
		SELECT id, host_id, provider, name, calendar_id, access_token, refresh_token,
		       token_expiry, caldav_url, caldav_username, caldav_password, is_default,
		       last_synced_at, COALESCE(sync_status, 'unknown'), COALESCE(sync_error, ''),
		       created_at, updated_at
		FROM calendar_connections WHERE host_id = $1
		ORDER BY is_default DESC, created_at ASC
	`)
	rows, err := r.db.QueryContext(ctx, query, hostID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var calendars []*models.CalendarConnection
	for rows.Next() {
		cal := &models.CalendarConnection{}
		err := rows.Scan(
			&cal.ID, &cal.HostID, &cal.Provider, &cal.Name, &cal.CalendarID,
			&cal.AccessToken, &cal.RefreshToken, &cal.TokenExpiry,
			&cal.CalDAVURL, &cal.CalDAVUsername, &cal.CalDAVPassword,
			&cal.IsDefault, &cal.LastSyncedAt, &cal.SyncStatus, &cal.SyncError,
			&cal.CreatedAt, &cal.UpdatedAt)
		if err != nil {
			return nil, err
		}
		calendars = append(calendars, cal)
	}
	return calendars, nil
}

func (r *CalendarRepository) Update(ctx context.Context, cal *models.CalendarConnection) error {
	query := q(r.driver, `
		UPDATE calendar_connections
		SET name = $1, access_token = $2, refresh_token = $3, token_expiry = $4,
		    caldav_url = $5, caldav_username = $6, caldav_password = $7, is_default = $8,
		    last_synced_at = $9, sync_status = $10, sync_error = $11
		WHERE id = $12
	`)
	_, err := r.db.ExecContext(ctx, query,
		cal.Name, cal.AccessToken, cal.RefreshToken, cal.TokenExpiry,
		cal.CalDAVURL, cal.CalDAVUsername, cal.CalDAVPassword,
		cal.IsDefault, cal.LastSyncedAt, cal.SyncStatus, cal.SyncError, cal.ID)
	return err
}

// UpdateSyncStatus updates only the sync status fields for a calendar
func (r *CalendarRepository) UpdateSyncStatus(ctx context.Context, calendarID string, syncStatus models.CalendarSyncStatus, syncError string, lastSyncedAt *models.SQLiteTime) error {
	query := q(r.driver, `
		UPDATE calendar_connections
		SET last_synced_at = $1, sync_status = $2, sync_error = $3
		WHERE id = $4
	`)
	_, err := r.db.ExecContext(ctx, query, lastSyncedAt, syncStatus, syncError, calendarID)
	return err
}

// GetAll returns all calendar connections across all hosts (for background sync)
func (r *CalendarRepository) GetAll(ctx context.Context) ([]*models.CalendarConnection, error) {
	query := q(r.driver, `
		SELECT id, host_id, provider, name, calendar_id, access_token, refresh_token,
		       token_expiry, caldav_url, caldav_username, caldav_password, is_default,
		       last_synced_at, COALESCE(sync_status, 'unknown'), COALESCE(sync_error, ''),
		       created_at, updated_at
		FROM calendar_connections
		ORDER BY created_at ASC
	`)
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var calendars []*models.CalendarConnection
	for rows.Next() {
		cal := &models.CalendarConnection{}
		err := rows.Scan(
			&cal.ID, &cal.HostID, &cal.Provider, &cal.Name, &cal.CalendarID,
			&cal.AccessToken, &cal.RefreshToken, &cal.TokenExpiry,
			&cal.CalDAVURL, &cal.CalDAVUsername, &cal.CalDAVPassword,
			&cal.IsDefault, &cal.LastSyncedAt, &cal.SyncStatus, &cal.SyncError,
			&cal.CreatedAt, &cal.UpdatedAt)
		if err != nil {
			return nil, err
		}
		calendars = append(calendars, cal)
	}
	return calendars, nil
}

func (r *CalendarRepository) Delete(ctx context.Context, id string) error {
	query := q(r.driver, `DELETE FROM calendar_connections WHERE id = $1`)
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *CalendarRepository) SetDefault(ctx context.Context, hostID, calendarID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	_, err = tx.ExecContext(ctx, q(r.driver, `UPDATE calendar_connections SET is_default = FALSE WHERE host_id = $1`), hostID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, q(r.driver, `UPDATE calendar_connections SET is_default = TRUE WHERE id = $1`), calendarID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ConferencingRepository handles conferencing connection database operations
type ConferencingRepository struct {
	db     *sql.DB
	driver string
}

func (r *ConferencingRepository) Create(ctx context.Context, conn *models.ConferencingConnection) error {
	query := q(r.driver, `
		INSERT INTO conferencing_connections (id, host_id, provider, access_token,
			refresh_token, token_expiry, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`)
	_, err := r.db.ExecContext(ctx, query,
		conn.ID, conn.HostID, conn.Provider, conn.AccessToken,
		conn.RefreshToken, conn.TokenExpiry, conn.CreatedAt, conn.UpdatedAt)
	return err
}

func (r *ConferencingRepository) GetByHostID(ctx context.Context, hostID string) ([]*models.ConferencingConnection, error) {
	query := q(r.driver, `
		SELECT id, host_id, provider, access_token, refresh_token, token_expiry,
		       created_at, updated_at
		FROM conferencing_connections WHERE host_id = $1
	`)
	rows, err := r.db.QueryContext(ctx, query, hostID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var connections []*models.ConferencingConnection
	for rows.Next() {
		conn := &models.ConferencingConnection{}
		err := rows.Scan(
			&conn.ID, &conn.HostID, &conn.Provider, &conn.AccessToken,
			&conn.RefreshToken, &conn.TokenExpiry, &conn.CreatedAt, &conn.UpdatedAt)
		if err != nil {
			return nil, err
		}
		connections = append(connections, conn)
	}
	return connections, nil
}

func (r *ConferencingRepository) GetByHostAndProvider(ctx context.Context, hostID string, provider models.ConferencingProvider) (*models.ConferencingConnection, error) {
	conn := &models.ConferencingConnection{}
	query := q(r.driver, `
		SELECT id, host_id, provider, access_token, refresh_token, token_expiry,
		       created_at, updated_at
		FROM conferencing_connections WHERE host_id = $1 AND provider = $2
	`)
	err := r.db.QueryRowContext(ctx, query, hostID, provider).Scan(
		&conn.ID, &conn.HostID, &conn.Provider, &conn.AccessToken,
		&conn.RefreshToken, &conn.TokenExpiry, &conn.CreatedAt, &conn.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return conn, err
}

func (r *ConferencingRepository) Update(ctx context.Context, conn *models.ConferencingConnection) error {
	query := q(r.driver, `
		UPDATE conferencing_connections
		SET access_token = $1, refresh_token = $2, token_expiry = $3
		WHERE id = $4
	`)
	_, err := r.db.ExecContext(ctx, query,
		conn.AccessToken, conn.RefreshToken, conn.TokenExpiry, conn.ID)
	return err
}

func (r *ConferencingRepository) Delete(ctx context.Context, id string) error {
	query := q(r.driver, `DELETE FROM conferencing_connections WHERE id = $1`)
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// TemplateRepository handles meeting template database operations
type TemplateRepository struct {
	db     *sql.DB
	driver string
}

func (r *TemplateRepository) Create(ctx context.Context, tmpl *models.MeetingTemplate) error {
	query := q(r.driver, `
		INSERT INTO meeting_templates (id, host_id, slug, name, description, durations,
			location_type, custom_location, calendar_id, requires_approval,
			min_notice_minutes, max_schedule_days, pre_buffer_minutes, post_buffer_minutes,
			availability_rules, invitee_questions, confirmation_email, reminder_email,
			is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
	`)
	_, err := r.db.ExecContext(ctx, query,
		tmpl.ID, tmpl.HostID, tmpl.Slug, tmpl.Name, tmpl.Description,
		tmpl.Durations, tmpl.LocationType, tmpl.CustomLocation, tmpl.CalendarID,
		tmpl.RequiresApproval, tmpl.MinNoticeMinutes, tmpl.MaxScheduleDays,
		tmpl.PreBufferMinutes, tmpl.PostBufferMinutes, tmpl.AvailabilityRules,
		tmpl.InviteeQuestions, tmpl.ConfirmationEmail, tmpl.ReminderEmail,
		tmpl.IsActive, tmpl.CreatedAt, tmpl.UpdatedAt)
	return err
}

func (r *TemplateRepository) GetByID(ctx context.Context, id string) (*models.MeetingTemplate, error) {
	tmpl := &models.MeetingTemplate{}
	query := q(r.driver, `
		SELECT id, host_id, slug, name, description, durations, location_type,
		       custom_location, calendar_id, requires_approval, min_notice_minutes,
		       max_schedule_days, pre_buffer_minutes, post_buffer_minutes,
		       availability_rules, invitee_questions, confirmation_email, reminder_email,
		       is_active, created_at, updated_at
		FROM meeting_templates WHERE id = $1
	`)
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&tmpl.ID, &tmpl.HostID, &tmpl.Slug, &tmpl.Name, &tmpl.Description,
		&tmpl.Durations, &tmpl.LocationType, &tmpl.CustomLocation, &tmpl.CalendarID,
		&tmpl.RequiresApproval, &tmpl.MinNoticeMinutes, &tmpl.MaxScheduleDays,
		&tmpl.PreBufferMinutes, &tmpl.PostBufferMinutes, &tmpl.AvailabilityRules,
		&tmpl.InviteeQuestions, &tmpl.ConfirmationEmail, &tmpl.ReminderEmail,
		&tmpl.IsActive, &tmpl.CreatedAt, &tmpl.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return tmpl, err
}

func (r *TemplateRepository) GetByHostAndSlug(ctx context.Context, hostID, slug string) (*models.MeetingTemplate, error) {
	tmpl := &models.MeetingTemplate{}
	query := q(r.driver, `
		SELECT id, host_id, slug, name, description, durations, location_type,
		       custom_location, calendar_id, requires_approval, min_notice_minutes,
		       max_schedule_days, pre_buffer_minutes, post_buffer_minutes,
		       availability_rules, invitee_questions, confirmation_email, reminder_email,
		       is_active, created_at, updated_at
		FROM meeting_templates WHERE host_id = $1 AND slug = $2
	`)
	err := r.db.QueryRowContext(ctx, query, hostID, slug).Scan(
		&tmpl.ID, &tmpl.HostID, &tmpl.Slug, &tmpl.Name, &tmpl.Description,
		&tmpl.Durations, &tmpl.LocationType, &tmpl.CustomLocation, &tmpl.CalendarID,
		&tmpl.RequiresApproval, &tmpl.MinNoticeMinutes, &tmpl.MaxScheduleDays,
		&tmpl.PreBufferMinutes, &tmpl.PostBufferMinutes, &tmpl.AvailabilityRules,
		&tmpl.InviteeQuestions, &tmpl.ConfirmationEmail, &tmpl.ReminderEmail,
		&tmpl.IsActive, &tmpl.CreatedAt, &tmpl.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return tmpl, err
}

func (r *TemplateRepository) GetByHostID(ctx context.Context, hostID string) ([]*models.MeetingTemplate, error) {
	query := q(r.driver, `
		SELECT id, host_id, slug, name, description, durations, location_type,
		       custom_location, calendar_id, requires_approval, min_notice_minutes,
		       max_schedule_days, pre_buffer_minutes, post_buffer_minutes,
		       availability_rules, invitee_questions, confirmation_email, reminder_email,
		       is_active, created_at, updated_at
		FROM meeting_templates WHERE host_id = $1
		ORDER BY created_at DESC
	`)
	rows, err := r.db.QueryContext(ctx, query, hostID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var templates []*models.MeetingTemplate
	for rows.Next() {
		tmpl := &models.MeetingTemplate{}
		err := rows.Scan(
			&tmpl.ID, &tmpl.HostID, &tmpl.Slug, &tmpl.Name, &tmpl.Description,
			&tmpl.Durations, &tmpl.LocationType, &tmpl.CustomLocation, &tmpl.CalendarID,
			&tmpl.RequiresApproval, &tmpl.MinNoticeMinutes, &tmpl.MaxScheduleDays,
			&tmpl.PreBufferMinutes, &tmpl.PostBufferMinutes, &tmpl.AvailabilityRules,
			&tmpl.InviteeQuestions, &tmpl.ConfirmationEmail, &tmpl.ReminderEmail,
			&tmpl.IsActive, &tmpl.CreatedAt, &tmpl.UpdatedAt)
		if err != nil {
			return nil, err
		}
		templates = append(templates, tmpl)
	}
	return templates, nil
}

func (r *TemplateRepository) Update(ctx context.Context, tmpl *models.MeetingTemplate) error {
	query := q(r.driver, `
		UPDATE meeting_templates
		SET slug = $1, name = $2, description = $3, durations = $4, location_type = $5,
		    custom_location = $6, calendar_id = $7, requires_approval = $8,
		    min_notice_minutes = $9, max_schedule_days = $10, pre_buffer_minutes = $11,
		    post_buffer_minutes = $12, availability_rules = $13, invitee_questions = $14,
		    confirmation_email = $15, reminder_email = $16, is_active = $17
		WHERE id = $18
	`)
	_, err := r.db.ExecContext(ctx, query,
		tmpl.Slug, tmpl.Name, tmpl.Description, tmpl.Durations, tmpl.LocationType,
		tmpl.CustomLocation, tmpl.CalendarID, tmpl.RequiresApproval,
		tmpl.MinNoticeMinutes, tmpl.MaxScheduleDays, tmpl.PreBufferMinutes,
		tmpl.PostBufferMinutes, tmpl.AvailabilityRules, tmpl.InviteeQuestions,
		tmpl.ConfirmationEmail, tmpl.ReminderEmail, tmpl.IsActive, tmpl.ID)
	return err
}

func (r *TemplateRepository) Delete(ctx context.Context, id string) error {
	query := q(r.driver, `DELETE FROM meeting_templates WHERE id = $1`)
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// BookingRepository handles booking database operations
type BookingRepository struct {
	db     *sql.DB
	driver string
}

func (r *BookingRepository) Create(ctx context.Context, booking *models.Booking) error {
	query := q(r.driver, `
		INSERT INTO bookings (id, template_id, host_id, token, status, start_time,
			end_time, duration, invitee_name, invitee_email, invitee_timezone,
			invitee_phone, additional_guests, answers, conference_link,
			calendar_event_id, reminder_sent, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`)
	_, err := r.db.ExecContext(ctx, query,
		booking.ID, booking.TemplateID, booking.HostID, booking.Token,
		booking.Status, booking.StartTime, booking.EndTime, booking.Duration,
		booking.InviteeName, booking.InviteeEmail, booking.InviteeTimezone,
		booking.InviteePhone, booking.AdditionalGuests, booking.Answers,
		booking.ConferenceLink, booking.CalendarEventID, booking.ReminderSent,
		booking.CreatedAt, booking.UpdatedAt)
	return err
}

func (r *BookingRepository) GetByID(ctx context.Context, id string) (*models.Booking, error) {
	booking := &models.Booking{}
	query := q(r.driver, `
		SELECT id, template_id, host_id, token, status, start_time, end_time, duration,
		       invitee_name, invitee_email, COALESCE(invitee_timezone, ''), COALESCE(invitee_phone, ''),
		       additional_guests, answers, COALESCE(conference_link, ''), COALESCE(calendar_event_id, ''),
		       COALESCE(cancelled_by, ''), COALESCE(cancel_reason, ''), COALESCE(reminder_sent, false),
		       created_at, updated_at
		FROM bookings WHERE id = $1
	`)
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&booking.ID, &booking.TemplateID, &booking.HostID, &booking.Token,
		&booking.Status, &booking.StartTime, &booking.EndTime, &booking.Duration,
		&booking.InviteeName, &booking.InviteeEmail, &booking.InviteeTimezone,
		&booking.InviteePhone, &booking.AdditionalGuests, &booking.Answers,
		&booking.ConferenceLink, &booking.CalendarEventID,
		&booking.CancelledBy, &booking.CancelReason, &booking.ReminderSent,
		&booking.CreatedAt, &booking.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return booking, err
}

func (r *BookingRepository) GetByToken(ctx context.Context, token string) (*models.Booking, error) {
	booking := &models.Booking{}
	query := q(r.driver, `
		SELECT id, template_id, host_id, token, status, start_time, end_time, duration,
		       invitee_name, invitee_email, COALESCE(invitee_timezone, ''), COALESCE(invitee_phone, ''),
		       additional_guests, answers, COALESCE(conference_link, ''), COALESCE(calendar_event_id, ''),
		       COALESCE(cancelled_by, ''), COALESCE(cancel_reason, ''), COALESCE(reminder_sent, false),
		       created_at, updated_at
		FROM bookings WHERE token = $1
	`)
	err := r.db.QueryRowContext(ctx, query, token).Scan(
		&booking.ID, &booking.TemplateID, &booking.HostID, &booking.Token,
		&booking.Status, &booking.StartTime, &booking.EndTime, &booking.Duration,
		&booking.InviteeName, &booking.InviteeEmail, &booking.InviteeTimezone,
		&booking.InviteePhone, &booking.AdditionalGuests, &booking.Answers,
		&booking.ConferenceLink, &booking.CalendarEventID,
		&booking.CancelledBy, &booking.CancelReason, &booking.ReminderSent,
		&booking.CreatedAt, &booking.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return booking, err
}

func (r *BookingRepository) GetByHostID(ctx context.Context, hostID string, status *models.BookingStatus) ([]*models.Booking, error) {
	var query string
	var args []interface{}

	if status != nil {
		query = q(r.driver, `
			SELECT id, template_id, host_id, token, status, start_time, end_time, duration,
			       invitee_name, invitee_email, COALESCE(invitee_timezone, ''), COALESCE(invitee_phone, ''),
			       additional_guests, answers, COALESCE(conference_link, ''), COALESCE(calendar_event_id, ''),
			       COALESCE(cancelled_by, ''), COALESCE(cancel_reason, ''), COALESCE(reminder_sent, false),
			       created_at, updated_at
			FROM bookings WHERE host_id = $1 AND status = $2
			ORDER BY start_time ASC
		`)
		args = []interface{}{hostID, *status}
	} else {
		query = q(r.driver, `
			SELECT id, template_id, host_id, token, status, start_time, end_time, duration,
			       invitee_name, invitee_email, COALESCE(invitee_timezone, ''), COALESCE(invitee_phone, ''),
			       additional_guests, answers, COALESCE(conference_link, ''), COALESCE(calendar_event_id, ''),
			       COALESCE(cancelled_by, ''), COALESCE(cancel_reason, ''), COALESCE(reminder_sent, false),
			       created_at, updated_at
			FROM bookings WHERE host_id = $1
			ORDER BY start_time ASC
		`)
		args = []interface{}{hostID}
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
			&booking.CreatedAt, &booking.UpdatedAt)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, booking)
	}
	return bookings, nil
}

func (r *BookingRepository) GetByHostIDAndTimeRange(ctx context.Context, hostID string, start, end time.Time) ([]*models.Booking, error) {
	query := q(r.driver, `
		SELECT id, template_id, host_id, token, status, start_time, end_time, duration,
		       invitee_name, invitee_email, invitee_timezone, invitee_phone,
		       additional_guests, answers, conference_link, calendar_event_id,
		       cancelled_by, cancel_reason, COALESCE(reminder_sent, false), created_at, updated_at
		FROM bookings
		WHERE host_id = $1
		  AND status IN ('pending', 'confirmed')
		  AND start_time < $3 AND end_time > $2
		ORDER BY start_time ASC
	`)
	rows, err := r.db.QueryContext(ctx, query, hostID, start, end)
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
			&booking.CreatedAt, &booking.UpdatedAt)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, booking)
	}
	return bookings, nil
}

func (r *BookingRepository) Update(ctx context.Context, booking *models.Booking) error {
	query := q(r.driver, `
		UPDATE bookings
		SET status = $1, conference_link = $2, calendar_event_id = $3,
		    cancelled_by = $4, cancel_reason = $5, reminder_sent = $6
		WHERE id = $7
	`)
	_, err := r.db.ExecContext(ctx, query,
		booking.Status, booking.ConferenceLink, booking.CalendarEventID,
		booking.CancelledBy, booking.CancelReason, booking.ReminderSent, booking.ID)
	return err
}

// GetBookingsNeedingReminder returns confirmed bookings that start within the given time range
// and haven't had a reminder sent yet
func (r *BookingRepository) GetBookingsNeedingReminder(ctx context.Context, start, end time.Time) ([]*models.Booking, error) {
	query := q(r.driver, `
		SELECT id, template_id, host_id, token, status, start_time, end_time, duration,
		       invitee_name, invitee_email, COALESCE(invitee_timezone, ''), COALESCE(invitee_phone, ''),
		       additional_guests, answers, COALESCE(conference_link, ''), COALESCE(calendar_event_id, ''),
		       COALESCE(cancelled_by, ''), COALESCE(cancel_reason, ''), COALESCE(reminder_sent, false),
		       created_at, updated_at
		FROM bookings
		WHERE status = 'confirmed'
		  AND (reminder_sent = false OR reminder_sent IS NULL)
		  AND start_time >= $1
		  AND start_time <= $2
		ORDER BY start_time ASC
	`)
	rows, err := r.db.QueryContext(ctx, query, start, end)
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
			&booking.CreatedAt, &booking.UpdatedAt)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, booking)
	}
	return bookings, nil
}

// MarkReminderSent marks a booking's reminder as sent
func (r *BookingRepository) MarkReminderSent(ctx context.Context, bookingID string) error {
	query := q(r.driver, `UPDATE bookings SET reminder_sent = $1 WHERE id = $2`)
	_, err := r.db.ExecContext(ctx, query, true, bookingID)
	return err
}

// BookingCount holds the count of bookings by status for a template
type BookingCount struct {
	TemplateID string
	Total      int
	Pending    int
	Confirmed  int
}

// GetBookingCountsByHostID returns booking counts grouped by template for a given host
func (r *BookingRepository) GetBookingCountsByHostID(ctx context.Context, hostID string) (map[string]*BookingCount, error) {
	query := q(r.driver, `
		SELECT template_id, status, COUNT(*) as count
		FROM bookings
		WHERE host_id = $1
		GROUP BY template_id, status
	`)
	rows, err := r.db.QueryContext(ctx, query, hostID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	counts := make(map[string]*BookingCount)
	for rows.Next() {
		var templateID string
		var status string
		var count int
		if err := rows.Scan(&templateID, &status, &count); err != nil {
			return nil, err
		}

		if _, ok := counts[templateID]; !ok {
			counts[templateID] = &BookingCount{TemplateID: templateID}
		}

		counts[templateID].Total += count
		switch status {
		case "pending":
			counts[templateID].Pending = count
		case "confirmed":
			counts[templateID].Confirmed = count
		}
	}

	return counts, nil
}

// SessionRepository handles session database operations
type SessionRepository struct {
	db     *sql.DB
	driver string
}

func (r *SessionRepository) Create(ctx context.Context, session *models.Session) error {
	query := q(r.driver, `
		INSERT INTO sessions (id, host_id, token, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`)
	_, err := r.db.ExecContext(ctx, query,
		session.ID, session.HostID, session.Token, session.ExpiresAt, session.CreatedAt)
	return err
}

func (r *SessionRepository) GetByToken(ctx context.Context, token string) (*models.Session, error) {
	session := &models.Session{}
	var query string
	if r.driver == "sqlite" {
		// Use strftime to get RFC3339 format for proper string comparison
		query = `SELECT id, host_id, token, expires_at, created_at FROM sessions WHERE token = ? AND expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now')`
	} else {
		query = `SELECT id, host_id, token, expires_at, created_at FROM sessions WHERE token = $1 AND expires_at > NOW()`
	}
	err := r.db.QueryRowContext(ctx, query, token).Scan(
		&session.ID, &session.HostID, &session.Token, &session.ExpiresAt, &session.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return session, err
}

func (r *SessionRepository) Delete(ctx context.Context, token string) error {
	query := q(r.driver, `DELETE FROM sessions WHERE token = $1`)
	_, err := r.db.ExecContext(ctx, query, token)
	return err
}

func (r *SessionRepository) DeleteExpired(ctx context.Context) error {
	var query string
	if r.driver == "sqlite" {
		// Use strftime to get RFC3339 format for proper string comparison
		query = `DELETE FROM sessions WHERE expires_at < strftime('%Y-%m-%dT%H:%M:%SZ', 'now')`
	} else {
		query = `DELETE FROM sessions WHERE expires_at < NOW()`
	}
	_, err := r.db.ExecContext(ctx, query)
	return err
}

// WorkingHoursRepository handles working hours database operations
type WorkingHoursRepository struct {
	db     *sql.DB
	driver string
}

func (r *WorkingHoursRepository) GetByHostID(ctx context.Context, hostID string) ([]*models.WorkingHours, error) {
	query := q(r.driver, `
		SELECT id, host_id, day_of_week, start_time, end_time, is_enabled, created_at, updated_at
		FROM working_hours WHERE host_id = $1
		ORDER BY day_of_week, start_time
	`)
	rows, err := r.db.QueryContext(ctx, query, hostID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var hours []*models.WorkingHours
	for rows.Next() {
		wh := &models.WorkingHours{}
		err := rows.Scan(
			&wh.ID, &wh.HostID, &wh.DayOfWeek, &wh.StartTime, &wh.EndTime,
			&wh.IsEnabled, &wh.CreatedAt, &wh.UpdatedAt)
		if err != nil {
			return nil, err
		}
		hours = append(hours, wh)
	}
	return hours, nil
}

func (r *WorkingHoursRepository) SetForHost(ctx context.Context, hostID string, hours []*models.WorkingHours) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	_, err = tx.ExecContext(ctx, q(r.driver, `DELETE FROM working_hours WHERE host_id = $1`), hostID)
	if err != nil {
		return err
	}

	insertQuery := q(r.driver, `
		INSERT INTO working_hours (id, host_id, day_of_week, start_time, end_time, is_enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`)
	for _, wh := range hours {
		_, err = tx.ExecContext(ctx, insertQuery,
			wh.ID, hostID, wh.DayOfWeek, wh.StartTime, wh.EndTime, wh.IsEnabled, wh.CreatedAt, wh.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to insert working hour: %w", err)
		}
	}

	return tx.Commit()
}

// AuditLogRepository handles audit log database operations
type AuditLogRepository struct {
	db     *sql.DB
	driver string
}

func (r *AuditLogRepository) Create(ctx context.Context, log *models.AuditLog) error {
	query := q(r.driver, `
		INSERT INTO audit_logs (id, tenant_id, host_id, action, entity_type, entity_id, details, ip_address, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`)
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.TenantID, log.HostID, log.Action, log.EntityType,
		log.EntityID, log.Details, log.IPAddress, log.CreatedAt)
	return err
}

func (r *AuditLogRepository) GetByTenantID(ctx context.Context, tenantID string, limit, offset int) ([]*models.AuditLog, error) {
	query := q(r.driver, `
		SELECT id, tenant_id, host_id, action, entity_type, entity_id, details, ip_address, created_at
		FROM audit_logs WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`)
	rows, err := r.db.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var logs []*models.AuditLog
	for rows.Next() {
		log := &models.AuditLog{}
		err := rows.Scan(
			&log.ID, &log.TenantID, &log.HostID, &log.Action, &log.EntityType,
			&log.EntityID, &log.Details, &log.IPAddress, &log.CreatedAt)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}

func (r *AuditLogRepository) CountByTenantID(ctx context.Context, tenantID string) (int, error) {
	query := q(r.driver, `SELECT COUNT(*) FROM audit_logs WHERE tenant_id = $1`)
	var count int
	err := r.db.QueryRowContext(ctx, query, tenantID).Scan(&count)
	return count, err
}
