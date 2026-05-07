-- Hosted events: host-driven scheduling (the inverse of bookings).
CREATE TABLE hosted_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    template_id UUID REFERENCES meeting_templates(id) ON DELETE SET NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    duration INTEGER NOT NULL,
    timezone VARCHAR(100) NOT NULL DEFAULT 'UTC',
    location_type VARCHAR(50) NOT NULL DEFAULT 'google_meet',
    custom_location VARCHAR(500) NOT NULL DEFAULT '',
    calendar_id VARCHAR(255) NOT NULL DEFAULT '',
    conference_link VARCHAR(1000) NOT NULL DEFAULT '',
    status VARCHAR(50) NOT NULL DEFAULT 'scheduled',
    cancel_reason TEXT NOT NULL DEFAULT '',
    reminder_sent BOOLEAN NOT NULL DEFAULT FALSE,
    is_archived BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_hosted_events_host_start ON hosted_events(host_id, start_time);
CREATE INDEX idx_hosted_events_tenant ON hosted_events(tenant_id);

CREATE TABLE hosted_event_attendees (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    hosted_event_id UUID NOT NULL REFERENCES hosted_events(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL DEFAULT '',
    contact_id UUID REFERENCES contacts(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(hosted_event_id, email)
);

CREATE INDEX idx_hosted_event_attendees_event ON hosted_event_attendees(hosted_event_id);

CREATE TABLE hosted_event_calendar_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    hosted_event_id UUID NOT NULL REFERENCES hosted_events(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    calendar_id UUID NOT NULL REFERENCES provider_calendars(id) ON DELETE CASCADE,
    event_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_hosted_event_calendar_events_item ON hosted_event_calendar_events(hosted_event_id);
