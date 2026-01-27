-- Remove is_private column from meeting_templates
DROP INDEX IF EXISTS idx_meeting_templates_private;
-- SQLite doesn't support DROP COLUMN directly, would need table recreation
-- For simplicity, we just drop the index - the column will be ignored
