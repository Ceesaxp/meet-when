-- Add template_hosts junction table for pooled hosts feature
CREATE TABLE template_hosts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES meeting_templates(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL DEFAULT 'sibling' CHECK (role IN ('owner', 'sibling')),
    is_optional BOOLEAN NOT NULL DEFAULT false,
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(template_id, host_id)
);

-- Add booking_calendar_events table to track events created per host
CREATE TABLE booking_calendar_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    host_id UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    calendar_id UUID NOT NULL REFERENCES calendar_connections(id) ON DELETE CASCADE,
    event_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for efficient lookups
CREATE INDEX idx_template_hosts_template_id ON template_hosts(template_id);
CREATE INDEX idx_template_hosts_host_id ON template_hosts(host_id);
CREATE INDEX idx_booking_calendar_events_booking_id ON booking_calendar_events(booking_id);

-- Trigger for updated_at
CREATE TRIGGER update_template_hosts_updated_at BEFORE UPDATE ON template_hosts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Data migration: populate template_hosts from existing templates
-- Each template's host_id becomes the owner
INSERT INTO template_hosts (id, template_id, host_id, role, is_optional, display_order, created_at, updated_at)
SELECT
    uuid_generate_v4() as id,
    mt.id as template_id,
    mt.host_id as host_id,
    'owner' as role,
    false as is_optional,
    0 as display_order,
    mt.created_at,
    mt.updated_at
FROM meeting_templates mt
WHERE NOT EXISTS (
    SELECT 1 FROM template_hosts th
    WHERE th.template_id = mt.id AND th.host_id = mt.host_id
);
