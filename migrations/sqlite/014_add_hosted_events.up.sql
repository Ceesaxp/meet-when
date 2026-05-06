-- Hosted events: host-driven scheduling (the inverse of bookings).
CREATE TABLE hosted_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    template_id TEXT REFERENCES meeting_templates(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    duration INTEGER NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    location_type TEXT NOT NULL DEFAULT 'google_meet',
    custom_location TEXT NOT NULL DEFAULT '',
    calendar_id TEXT NOT NULL DEFAULT '',
    conference_link TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'scheduled',
    cancel_reason TEXT NOT NULL DEFAULT '',
    reminder_sent INTEGER NOT NULL DEFAULT 0,
    is_archived INTEGER NOT NULL DEFAULT 0,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_hosted_events_host_start ON hosted_events(host_id, start_time);
CREATE INDEX idx_hosted_events_tenant ON hosted_events(tenant_id);

CREATE TABLE hosted_event_attendees (
    id TEXT PRIMARY KEY,
    hosted_event_id TEXT NOT NULL REFERENCES hosted_events(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    contact_id TEXT REFERENCES contacts(id) ON DELETE SET NULL,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(hosted_event_id, email)
);

CREATE INDEX idx_hosted_event_attendees_event ON hosted_event_attendees(hosted_event_id);

CREATE TABLE hosted_event_calendar_events (
    id TEXT PRIMARY KEY,
    hosted_event_id TEXT NOT NULL REFERENCES hosted_events(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    calendar_id TEXT NOT NULL REFERENCES provider_calendars(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_hosted_event_calendar_events_item ON hosted_event_calendar_events(hosted_event_id);
