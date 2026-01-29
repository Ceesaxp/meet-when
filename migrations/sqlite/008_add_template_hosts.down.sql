-- Rollback template_hosts and booking_calendar_events tables
DROP INDEX IF EXISTS idx_booking_calendar_events_booking_id;
DROP INDEX IF EXISTS idx_template_hosts_host_id;
DROP INDEX IF EXISTS idx_template_hosts_template_id;
DROP TABLE IF EXISTS booking_calendar_events;
DROP TABLE IF EXISTS template_hosts;
