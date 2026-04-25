-- Add smart_durations column to hosts table
ALTER TABLE hosts ADD COLUMN smart_durations BOOLEAN NOT NULL DEFAULT false;
