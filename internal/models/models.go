package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// SQLiteTime is a time.Time wrapper that can scan SQLite datetime strings
type SQLiteTime struct {
	time.Time
}

// Scan implements sql.Scanner for SQLiteTime
func (st *SQLiteTime) Scan(value interface{}) error {
	if value == nil {
		st.Time = time.Time{}
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		st.Time = v
		return nil
	case string:
		// Try various formats
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999-07:00",
			"2006-01-02 15:04:05-07:00",
			"2006-01-02 15:04:05",
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, v); err == nil {
				st.Time = t
				return nil
			}
		}
		return errors.New("unable to parse time: " + v)
	default:
		return errors.New("unsupported type for SQLiteTime")
	}
}

// Value implements driver.Valuer for SQLiteTime
func (st SQLiteTime) Value() (driver.Value, error) {
	// Always store in UTC with Z suffix for consistent string comparisons in SQLite
	return st.Time.UTC().Format("2006-01-02T15:04:05Z"), nil
}

// Now returns the current time as SQLiteTime (in UTC)
func Now() SQLiteTime {
	return SQLiteTime{Time: time.Now().UTC()}
}

// NewSQLiteTime creates a SQLiteTime from a time.Time (converted to UTC)
func NewSQLiteTime(t time.Time) SQLiteTime {
	return SQLiteTime{Time: t.UTC()}
}

// Tenant represents a multi-tenant organization
type Tenant struct {
	ID        string     `json:"id" db:"id"`
	Slug      string     `json:"slug" db:"slug"`
	Name      string     `json:"name" db:"name"`
	CreatedAt SQLiteTime `json:"created_at" db:"created_at"`
	UpdatedAt SQLiteTime `json:"updated_at" db:"updated_at"`
}

// Host represents a user who can receive bookings
type Host struct {
	ID                  string     `json:"id" db:"id"`
	TenantID            string     `json:"tenant_id" db:"tenant_id"`
	Email               string     `json:"email" db:"email"`
	PasswordHash        string     `json:"-" db:"password_hash"`
	Name                string     `json:"name" db:"name"`
	Slug                string     `json:"slug" db:"slug"`
	Timezone            string     `json:"timezone" db:"timezone"`
	DefaultCalendarID   *string    `json:"default_calendar_id" db:"default_calendar_id"`
	IsAdmin             bool       `json:"is_admin" db:"is_admin"`
	OnboardingCompleted bool       `json:"onboarding_completed" db:"onboarding_completed"`
	CreatedAt           SQLiteTime `json:"created_at" db:"created_at"`
	UpdatedAt           SQLiteTime `json:"updated_at" db:"updated_at"`
}

// WorkingHours represents the host's available working hours
type WorkingHours struct {
	ID        string     `json:"id" db:"id"`
	HostID    string     `json:"host_id" db:"host_id"`
	DayOfWeek int        `json:"day_of_week" db:"day_of_week"` // 0=Sunday, 6=Saturday
	StartTime string     `json:"start_time" db:"start_time"`   // HH:MM format
	EndTime   string     `json:"end_time" db:"end_time"`       // HH:MM format
	IsEnabled bool       `json:"is_enabled" db:"is_enabled"`
	CreatedAt SQLiteTime `json:"created_at" db:"created_at"`
	UpdatedAt SQLiteTime `json:"updated_at" db:"updated_at"`
}

// CalendarProvider represents supported calendar providers
type CalendarProvider string

const (
	CalendarProviderGoogle CalendarProvider = "google"
	CalendarProviderICloud CalendarProvider = "icloud"
	CalendarProviderCalDAV CalendarProvider = "caldav"
)

// CalendarSyncStatus represents the sync status of a calendar
type CalendarSyncStatus string

const (
	CalendarSyncStatusUnknown CalendarSyncStatus = "unknown"
	CalendarSyncStatusSynced  CalendarSyncStatus = "synced"
	CalendarSyncStatusFailed  CalendarSyncStatus = "failed"
)

// CalendarConnection represents a connected calendar
type CalendarConnection struct {
	ID           string           `json:"id" db:"id"`
	HostID       string           `json:"host_id" db:"host_id"`
	Provider     CalendarProvider `json:"provider" db:"provider"`
	Name         string           `json:"name" db:"name"`
	CalendarID   string           `json:"calendar_id" db:"calendar_id"`
	AccessToken  string           `json:"-" db:"access_token"`
	RefreshToken string           `json:"-" db:"refresh_token"`
	TokenExpiry  *SQLiteTime      `json:"-" db:"token_expiry"`
	// CalDAV specific
	CalDAVURL      string `json:"-" db:"caldav_url"`
	CalDAVUsername string `json:"-" db:"caldav_username"`
	CalDAVPassword string `json:"-" db:"caldav_password"`
	IsDefault      bool   `json:"is_default" db:"is_default"`
	// Sync status tracking
	LastSyncedAt *SQLiteTime        `json:"last_synced_at" db:"last_synced_at"`
	SyncStatus   CalendarSyncStatus `json:"sync_status" db:"sync_status"`
	SyncError    string             `json:"sync_error" db:"sync_error"`
	CreatedAt    SQLiteTime         `json:"created_at" db:"created_at"`
	UpdatedAt    SQLiteTime         `json:"updated_at" db:"updated_at"`
}

// ConferencingProvider represents supported conferencing providers
type ConferencingProvider string

const (
	ConferencingProviderGoogleMeet ConferencingProvider = "google_meet"
	ConferencingProviderZoom       ConferencingProvider = "zoom"
	ConferencingProviderPhone      ConferencingProvider = "phone"
	ConferencingProviderCustom     ConferencingProvider = "custom"
)

// ConferencingConnection represents a connected conferencing provider
type ConferencingConnection struct {
	ID           string               `json:"id" db:"id"`
	HostID       string               `json:"host_id" db:"host_id"`
	Provider     ConferencingProvider `json:"provider" db:"provider"`
	AccessToken  string               `json:"-" db:"access_token"`
	RefreshToken string               `json:"-" db:"refresh_token"`
	TokenExpiry  *SQLiteTime          `json:"-" db:"token_expiry"`
	CreatedAt    SQLiteTime           `json:"created_at" db:"created_at"`
	UpdatedAt    SQLiteTime           `json:"updated_at" db:"updated_at"`
}

// MeetingTemplate represents a bookable meeting type
type MeetingTemplate struct {
	ID                string               `json:"id" db:"id"`
	HostID            string               `json:"host_id" db:"host_id"`
	Slug              string               `json:"slug" db:"slug"`
	Name              string               `json:"name" db:"name"`
	Description       string               `json:"description" db:"description"`
	Durations         IntSlice             `json:"durations" db:"durations"` // Minutes, e.g., [30, 60]
	LocationType      ConferencingProvider `json:"location_type" db:"location_type"`
	CustomLocation    string               `json:"custom_location" db:"custom_location"`
	CalendarID        string               `json:"calendar_id" db:"calendar_id"` // Which calendar to write to
	RequiresApproval  bool                 `json:"requires_approval" db:"requires_approval"`
	MinNoticeMinutes  int                  `json:"min_notice_minutes" db:"min_notice_minutes"`
	MaxScheduleDays   int                  `json:"max_schedule_days" db:"max_schedule_days"`
	PreBufferMinutes  int                  `json:"pre_buffer_minutes" db:"pre_buffer_minutes"`
	PostBufferMinutes int                  `json:"post_buffer_minutes" db:"post_buffer_minutes"`
	AvailabilityRules JSONMap              `json:"availability_rules" db:"availability_rules"`
	InviteeQuestions  JSONArray            `json:"invitee_questions" db:"invitee_questions"`
	ConfirmationEmail string               `json:"confirmation_email" db:"confirmation_email"`
	ReminderEmail     string               `json:"reminder_email" db:"reminder_email"`
	IsActive          bool                 `json:"is_active" db:"is_active"`
	CreatedAt         SQLiteTime           `json:"created_at" db:"created_at"`
	UpdatedAt         SQLiteTime           `json:"updated_at" db:"updated_at"`
}

// BookingStatus represents the status of a booking
type BookingStatus string

const (
	BookingStatusPending   BookingStatus = "pending"
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
	BookingStatusRejected  BookingStatus = "rejected"
)

// Booking represents a scheduled meeting
type Booking struct {
	ID               string        `json:"id" db:"id"`
	TemplateID       string        `json:"template_id" db:"template_id"`
	HostID           string        `json:"host_id" db:"host_id"`
	Token            string        `json:"token" db:"token"` // For public access/cancel links
	Status           BookingStatus `json:"status" db:"status"`
	StartTime        SQLiteTime    `json:"start_time" db:"start_time"`
	EndTime          SQLiteTime    `json:"end_time" db:"end_time"`
	Duration         int           `json:"duration" db:"duration"` // Minutes
	InviteeName      string        `json:"invitee_name" db:"invitee_name"`
	InviteeEmail     string        `json:"invitee_email" db:"invitee_email"`
	InviteeTimezone  string        `json:"invitee_timezone" db:"invitee_timezone"`
	InviteePhone     string        `json:"invitee_phone" db:"invitee_phone"`
	AdditionalGuests StringSlice   `json:"additional_guests" db:"additional_guests"`
	Answers          JSONMap       `json:"answers" db:"answers"`
	ConferenceLink   string        `json:"conference_link" db:"conference_link"`
	CalendarEventID  string        `json:"calendar_event_id" db:"calendar_event_id"`
	CancelledBy      string        `json:"cancelled_by" db:"cancelled_by"` // host or invitee
	CancelReason     string        `json:"cancel_reason" db:"cancel_reason"`
	ReminderSent     bool          `json:"reminder_sent" db:"reminder_sent"`
	CreatedAt        SQLiteTime    `json:"created_at" db:"created_at"`
	UpdatedAt        SQLiteTime    `json:"updated_at" db:"updated_at"`
}

// Session represents a user session
type Session struct {
	ID        string     `json:"id" db:"id"`
	HostID    string     `json:"host_id" db:"host_id"`
	Token     string     `json:"-" db:"token"`
	ExpiresAt SQLiteTime `json:"expires_at" db:"expires_at"`
	CreatedAt SQLiteTime `json:"created_at" db:"created_at"`
}

// AuditLog represents an audit trail entry
type AuditLog struct {
	ID         string     `json:"id" db:"id"`
	TenantID   string     `json:"tenant_id" db:"tenant_id"`
	HostID     *string    `json:"host_id" db:"host_id"`
	Action     string     `json:"action" db:"action"`
	EntityType string     `json:"entity_type" db:"entity_type"`
	EntityID   string     `json:"entity_id" db:"entity_id"`
	Details    JSONMap    `json:"details" db:"details"`
	IPAddress  string     `json:"ip_address" db:"ip_address"`
	CreatedAt  SQLiteTime `json:"created_at" db:"created_at"`
}

// TimeSlot represents an available time slot
type TimeSlot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Custom JSON types for PostgreSQL arrays and JSONB

// IntSlice is a slice of integers that can be stored as JSONB
type IntSlice []int

func (s IntSlice) Value() (driver.Value, error) {
	return json.Marshal(s)
}

func (s *IntSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, s)
}

// StringSlice is a slice of strings that can be stored as JSONB
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	return json.Marshal(s)
}

func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, s)
}

// JSONMap is a map that can be stored as JSONB
type JSONMap map[string]interface{}

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

func (m *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, m)
}

// JSONArray is an array that can be stored as JSONB
type JSONArray []interface{}

func (a JSONArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return json.Marshal(a)
}

func (a *JSONArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, a)
}
