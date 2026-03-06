-- Add smart_durations column to hosts table
ALTER TABLE hosts ADD COLUMN smart_durations INTEGER NOT NULL DEFAULT 0;
