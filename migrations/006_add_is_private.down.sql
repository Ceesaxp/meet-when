-- Remove is_private column from meeting_templates
DROP INDEX IF EXISTS idx_meeting_templates_private;
ALTER TABLE meeting_templates DROP COLUMN is_private;
