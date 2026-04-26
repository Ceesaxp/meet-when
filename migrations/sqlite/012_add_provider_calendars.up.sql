-- Add provider_calendars table for multiple calendars per provider connection.
-- See migrations/012_add_provider_calendars.up.sql for design notes.

-- Defer foreign-key checks until the transaction commits so the table-recreate
-- dance below can complete without temporarily violating bookings.template_id FKs.
PRAGMA defer_foreign_keys = 1;

CREATE TABLE provider_calendars (
    id TEXT PRIMARY KEY,
    connection_id TEXT NOT NULL REFERENCES calendar_connections(id) ON DELETE CASCADE,
    provider_calendar_id TEXT NOT NULL,
    name TEXT NOT NULL,
    color TEXT NOT NULL DEFAULT '',
    is_primary INTEGER NOT NULL DEFAULT 0,
    is_writable INTEGER NOT NULL DEFAULT 1,
    poll_busy INTEGER NOT NULL DEFAULT 1,
    last_synced_at TEXT,
    sync_status TEXT NOT NULL DEFAULT 'unknown',
    sync_error TEXT NOT NULL DEFAULT '',
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(connection_id, provider_calendar_id)
);

CREATE INDEX idx_provider_calendars_connection ON provider_calendars(connection_id);

-- Backfill: one provider_calendars row per existing calendar_connections row.
-- The id is reused so that all existing FK references remain valid after the
-- repoint below.
INSERT INTO provider_calendars (
    id, connection_id, provider_calendar_id, name, color, is_primary, is_writable,
    poll_busy, last_synced_at, sync_status, sync_error, created_at, updated_at
)
SELECT
    id,
    id,
    COALESCE(calendar_id, ''),
    name,
    COALESCE(color, ''),
    1,
    1,
    1,
    last_synced_at,
    COALESCE(sync_status, 'unknown'),
    COALESCE(sync_error, ''),
    created_at,
    updated_at
FROM calendar_connections;

-- SQLite cannot ALTER a foreign-key constraint, so we recreate the affected
-- tables with the FK now pointing at provider_calendars(id).

CREATE TABLE meeting_templates_new (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    durations TEXT NOT NULL DEFAULT '[30]',
    location_type TEXT NOT NULL DEFAULT 'google_meet',
    custom_location TEXT,
    calendar_id TEXT REFERENCES provider_calendars(id) ON DELETE SET NULL,
    requires_approval INTEGER DEFAULT 1,
    min_notice_minutes INTEGER DEFAULT 60,
    max_schedule_days INTEGER DEFAULT 14,
    pre_buffer_minutes INTEGER DEFAULT 0,
    post_buffer_minutes INTEGER DEFAULT 0,
    availability_rules TEXT DEFAULT '{}',
    invitee_questions TEXT DEFAULT '[]',
    confirmation_email TEXT,
    reminder_email TEXT,
    is_active INTEGER DEFAULT 1,
    is_private INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(host_id, slug)
);

INSERT INTO meeting_templates_new (
    id, host_id, slug, name, description, durations, location_type, custom_location,
    calendar_id, requires_approval, min_notice_minutes, max_schedule_days,
    pre_buffer_minutes, post_buffer_minutes, availability_rules, invitee_questions,
    confirmation_email, reminder_email, is_active, is_private, created_at, updated_at
)
SELECT
    id, host_id, slug, name, description, durations, location_type, custom_location,
    calendar_id, requires_approval, min_notice_minutes, max_schedule_days,
    pre_buffer_minutes, post_buffer_minutes, availability_rules, invitee_questions,
    confirmation_email, reminder_email, is_active, COALESCE(is_private, 0), created_at, updated_at
FROM meeting_templates;

DROP TABLE meeting_templates;
ALTER TABLE meeting_templates_new RENAME TO meeting_templates;

CREATE INDEX idx_meeting_templates_host ON meeting_templates(host_id);
CREATE INDEX idx_meeting_templates_slug ON meeting_templates(slug);
CREATE INDEX idx_meeting_templates_private ON meeting_templates(host_id, is_private);

CREATE TABLE booking_calendar_events_new (
    id TEXT PRIMARY KEY,
    booking_id TEXT NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    calendar_id TEXT NOT NULL REFERENCES provider_calendars(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

INSERT INTO booking_calendar_events_new (id, booking_id, host_id, calendar_id, event_id, created_at)
SELECT id, booking_id, host_id, calendar_id, event_id, created_at
FROM booking_calendar_events;

DROP TABLE booking_calendar_events;
ALTER TABLE booking_calendar_events_new RENAME TO booking_calendar_events;

CREATE INDEX idx_booking_calendar_events_booking_id ON booking_calendar_events(booking_id);
