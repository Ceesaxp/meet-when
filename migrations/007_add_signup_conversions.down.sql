-- Drop signup conversions table and related objects (PostgreSQL version)
DROP TRIGGER IF EXISTS update_signup_conversions_updated_at ON signup_conversions;
DROP INDEX IF EXISTS idx_signup_conversions_clicked_at;
DROP INDEX IF EXISTS idx_signup_conversions_booking;
DROP INDEX IF EXISTS idx_signup_conversions_tenant;
DROP INDEX IF EXISTS idx_signup_conversions_invitee_email;
DROP TABLE IF EXISTS signup_conversions;
