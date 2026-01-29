-- Add template_hosts junction table for pooled hosts feature
CREATE TABLE template_hosts (
    id TEXT PRIMARY KEY,
    template_id TEXT NOT NULL REFERENCES meeting_templates(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'sibling' CHECK (role IN ('owner', 'sibling')),
    is_optional INTEGER NOT NULL DEFAULT 0,
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(template_id, host_id)
);

-- Add booking_calendar_events table to track events created per host
CREATE TABLE booking_calendar_events (
    id TEXT PRIMARY KEY,
    booking_id TEXT NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    calendar_id TEXT NOT NULL REFERENCES calendar_connections(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Indexes for efficient lookups
CREATE INDEX idx_template_hosts_template_id ON template_hosts(template_id);
CREATE INDEX idx_template_hosts_host_id ON template_hosts(host_id);
CREATE INDEX idx_booking_calendar_events_booking_id ON booking_calendar_events(booking_id);

-- Data migration: populate template_hosts from existing templates
-- Each template's host_id becomes the owner
INSERT INTO template_hosts (id, template_id, host_id, role, is_optional, display_order, created_at, updated_at)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' ||
    substr(lower(hex(randomblob(2))),2) || '-' ||
    substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' ||
    lower(hex(randomblob(6))) as id,
    mt.id as template_id,
    mt.host_id as host_id,
    'owner' as role,
    0 as is_optional,
    0 as display_order,
    mt.created_at,
    mt.updated_at
FROM meeting_templates mt
WHERE NOT EXISTS (
    SELECT 1 FROM template_hosts th
    WHERE th.template_id = mt.id AND th.host_id = mt.host_id
);
