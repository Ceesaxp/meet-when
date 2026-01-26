-- Tenants table
CREATE TABLE tenants (
    id TEXT PRIMARY KEY,
    slug TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_tenants_slug ON tenants(slug);

-- Hosts (users) table
CREATE TABLE hosts (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    timezone TEXT DEFAULT 'UTC',
    default_calendar_id TEXT,
    is_admin INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(tenant_id, email),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX idx_hosts_tenant ON hosts(tenant_id);
CREATE INDEX idx_hosts_email ON hosts(email);

-- Working hours table
CREATE TABLE working_hours (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    day_of_week INTEGER NOT NULL CHECK (day_of_week >= 0 AND day_of_week <= 6),
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    is_enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(host_id, day_of_week, start_time)
);

CREATE INDEX idx_working_hours_host ON working_hours(host_id);

-- Calendar connections table
CREATE TABLE calendar_connections (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    name TEXT NOT NULL,
    calendar_id TEXT,
    access_token TEXT,
    refresh_token TEXT,
    token_expiry TEXT,
    caldav_url TEXT,
    caldav_username TEXT,
    caldav_password TEXT,
    is_default INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_calendar_connections_host ON calendar_connections(host_id);

-- Conferencing connections table
CREATE TABLE conferencing_connections (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    access_token TEXT,
    refresh_token TEXT,
    token_expiry TEXT,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(host_id, provider)
);

CREATE INDEX idx_conferencing_connections_host ON conferencing_connections(host_id);

-- Meeting templates table
CREATE TABLE meeting_templates (
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
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(host_id, slug)
);

CREATE INDEX idx_meeting_templates_host ON meeting_templates(host_id);
CREATE INDEX idx_meeting_templates_slug ON meeting_templates(slug);

-- Bookings table
CREATE TABLE bookings (
    id TEXT PRIMARY KEY,
    template_id TEXT NOT NULL REFERENCES meeting_templates(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    token TEXT UNIQUE NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    duration INTEGER NOT NULL,
    invitee_name TEXT NOT NULL,
    invitee_email TEXT NOT NULL,
    invitee_timezone TEXT,
    invitee_phone TEXT,
    additional_guests TEXT DEFAULT '[]',
    answers TEXT DEFAULT '{}',
    conference_link TEXT,
    calendar_event_id TEXT,
    cancelled_by TEXT,
    cancel_reason TEXT,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_bookings_template ON bookings(template_id);
CREATE INDEX idx_bookings_host ON bookings(host_id);
CREATE INDEX idx_bookings_token ON bookings(token);
CREATE INDEX idx_bookings_status ON bookings(status);
CREATE INDEX idx_bookings_time ON bookings(start_time, end_time);
CREATE INDEX idx_bookings_invitee_email ON bookings(invitee_email);

-- Sessions table
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    token TEXT UNIQUE NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_sessions_token ON sessions(token);
CREATE INDEX idx_sessions_host ON sessions(host_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- Audit log table
CREATE TABLE audit_logs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    host_id TEXT REFERENCES hosts(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT,
    details TEXT DEFAULT '{}',
    ip_address TEXT,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_audit_logs_tenant ON audit_logs(tenant_id);
CREATE INDEX idx_audit_logs_host ON audit_logs(host_id);
CREATE INDEX idx_audit_logs_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_audit_logs_created ON audit_logs(created_at);
