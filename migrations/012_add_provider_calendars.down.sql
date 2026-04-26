-- Reverse 012_add_provider_calendars: drop provider_calendars and re-point
-- the FK constraints back at calendar_connections(id).
--
-- Rollback assumes that, for every meeting_templates.calendar_id /
-- hosts.default_calendar_id / booking_calendar_events.calendar_id value, a
-- corresponding calendar_connections.id row exists. That is true when
-- (a) the database has only ever held backfilled rows, or (b) any rows
-- created since 012 still point at primary calendars (whose id == the
-- connection id by construction). It is NOT true once a host has selected a
-- non-primary sub-calendar in a template; running this rollback in that
-- state will fail the FK re-add. Audit before rolling back in production.

ALTER TABLE meeting_templates
    DROP CONSTRAINT IF EXISTS meeting_templates_calendar_id_fkey;
ALTER TABLE meeting_templates
    ADD CONSTRAINT meeting_templates_calendar_id_fkey
        FOREIGN KEY (calendar_id) REFERENCES calendar_connections(id) ON DELETE SET NULL;

ALTER TABLE booking_calendar_events
    DROP CONSTRAINT IF EXISTS booking_calendar_events_calendar_id_fkey;
ALTER TABLE booking_calendar_events
    ADD CONSTRAINT booking_calendar_events_calendar_id_fkey
        FOREIGN KEY (calendar_id) REFERENCES calendar_connections(id) ON DELETE CASCADE;

ALTER TABLE hosts
    DROP CONSTRAINT IF EXISTS fk_hosts_default_calendar;
ALTER TABLE hosts
    ADD CONSTRAINT fk_hosts_default_calendar
        FOREIGN KEY (default_calendar_id) REFERENCES calendar_connections(id) ON DELETE SET NULL;

DROP TRIGGER IF EXISTS update_provider_calendars_updated_at ON provider_calendars;

DROP TABLE IF EXISTS provider_calendars;
