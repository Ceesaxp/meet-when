-- Add provider_calendars table to model multiple calendars per provider connection.
-- Each calendar_connections row is now treated as an account-level binding (OAuth or
-- CalDAV credentials); the calendars actually displayed and polled for busy times
-- live in provider_calendars.

CREATE TABLE provider_calendars (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    connection_id UUID NOT NULL REFERENCES calendar_connections(id) ON DELETE CASCADE,
    provider_calendar_id VARCHAR(500) NOT NULL,
    name VARCHAR(255) NOT NULL,
    color VARCHAR(7) NOT NULL DEFAULT '',
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    is_writable BOOLEAN NOT NULL DEFAULT TRUE,
    poll_busy BOOLEAN NOT NULL DEFAULT TRUE,
    last_synced_at TIMESTAMP WITH TIME ZONE,
    sync_status VARCHAR(20) NOT NULL DEFAULT 'unknown',
    sync_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(connection_id, provider_calendar_id)
);

CREATE INDEX idx_provider_calendars_connection ON provider_calendars(connection_id);

-- Backfill: insert one provider_calendars row per existing calendar_connections row.
-- We deliberately reuse the connection's UUID as the provider_calendars.id so that
-- existing FK references (meeting_templates.calendar_id, hosts.default_calendar_id,
-- booking_calendar_events.calendar_id) continue to point at a valid row after we
-- repoint the FKs in this migration.
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
    TRUE,
    TRUE,
    TRUE,
    last_synced_at,
    COALESCE(sync_status, 'unknown'),
    COALESCE(sync_error, ''),
    created_at,
    updated_at
FROM calendar_connections;

-- Repoint foreign keys from calendar_connections(id) to provider_calendars(id).
ALTER TABLE meeting_templates
    DROP CONSTRAINT IF EXISTS meeting_templates_calendar_id_fkey;
ALTER TABLE meeting_templates
    ADD CONSTRAINT meeting_templates_calendar_id_fkey
        FOREIGN KEY (calendar_id) REFERENCES provider_calendars(id) ON DELETE SET NULL;

ALTER TABLE booking_calendar_events
    DROP CONSTRAINT IF EXISTS booking_calendar_events_calendar_id_fkey;
ALTER TABLE booking_calendar_events
    ADD CONSTRAINT booking_calendar_events_calendar_id_fkey
        FOREIGN KEY (calendar_id) REFERENCES provider_calendars(id) ON DELETE CASCADE;

ALTER TABLE hosts
    DROP CONSTRAINT IF EXISTS fk_hosts_default_calendar;
ALTER TABLE hosts
    ADD CONSTRAINT fk_hosts_default_calendar
        FOREIGN KEY (default_calendar_id) REFERENCES provider_calendars(id) ON DELETE SET NULL;

CREATE TRIGGER update_provider_calendars_updated_at BEFORE UPDATE ON provider_calendars
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
