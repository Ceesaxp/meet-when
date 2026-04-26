-- Reverse 012_add_provider_calendars on SQLite. See the Postgres .down.sql for
-- the safety caveat: this rollback only succeeds while every recorded
-- calendar_id in meeting_templates / booking_calendar_events / hosts maps back
-- to a calendar_connections row.

PRAGMA defer_foreign_keys = 1;

CREATE TABLE meeting_templates_old (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    durations TEXT NOT NULL DEFAULT '[30]',
    location_type TEXT NOT NULL DEFAULT 'google_meet',
    custom_location TEXT,
    calendar_id TEXT REFERENCES calendar_connections(id) ON DELETE SET NULL,
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

INSERT INTO meeting_templates_old (
    id, host_id, slug, name, description, durations, location_type, custom_location,
    calendar_id, requires_approval, min_notice_minutes, max_schedule_days,
    pre_buffer_minutes, post_buffer_minutes, availability_rules, invitee_questions,
    confirmation_email, reminder_email, is_active, is_private, created_at, updated_at
)
SELECT
    id, host_id, slug, name, description, durations, location_type, custom_location,
    calendar_id, requires_approval, min_notice_minutes, max_schedule_days,
    pre_buffer_minutes, post_buffer_minutes, availability_rules, invitee_questions,
    confirmation_email, reminder_email, is_active, is_private, created_at, updated_at
FROM meeting_templates;

DROP TABLE meeting_templates;
ALTER TABLE meeting_templates_old RENAME TO meeting_templates;

CREATE INDEX idx_meeting_templates_host ON meeting_templates(host_id);
CREATE INDEX idx_meeting_templates_slug ON meeting_templates(slug);
CREATE INDEX idx_meeting_templates_private ON meeting_templates(host_id, is_private);

CREATE TABLE booking_calendar_events_old (
    id TEXT PRIMARY KEY,
    booking_id TEXT NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    calendar_id TEXT NOT NULL REFERENCES calendar_connections(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

INSERT INTO booking_calendar_events_old (id, booking_id, host_id, calendar_id, event_id, created_at)
SELECT id, booking_id, host_id, calendar_id, event_id, created_at
FROM booking_calendar_events;

DROP TABLE booking_calendar_events;
ALTER TABLE booking_calendar_events_old RENAME TO booking_calendar_events;

CREATE INDEX idx_booking_calendar_events_booking_id ON booking_calendar_events(booking_id);

DROP INDEX IF EXISTS idx_provider_calendars_connection;
DROP TABLE IF EXISTS provider_calendars;
