-- Add onboarding_completed field to hosts table
ALTER TABLE hosts ADD COLUMN onboarding_completed BOOLEAN DEFAULT FALSE;
