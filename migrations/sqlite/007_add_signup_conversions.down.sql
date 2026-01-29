-- Drop signup conversions table and related objects (SQLite version)
DROP INDEX IF EXISTS idx_signup_conversions_clicked_at;
DROP INDEX IF EXISTS idx_signup_conversions_booking;
DROP INDEX IF EXISTS idx_signup_conversions_tenant;
DROP INDEX IF EXISTS idx_signup_conversions_invitee_email;
DROP TABLE IF EXISTS signup_conversions;
