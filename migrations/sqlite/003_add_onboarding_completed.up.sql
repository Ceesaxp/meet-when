-- Add onboarding_completed field to hosts table
ALTER TABLE hosts ADD COLUMN onboarding_completed INTEGER DEFAULT 0;
