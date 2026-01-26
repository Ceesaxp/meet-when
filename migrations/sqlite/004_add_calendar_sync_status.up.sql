-- Add sync status fields to calendar_connections table
ALTER TABLE calendar_connections ADD COLUMN last_synced_at TEXT;
ALTER TABLE calendar_connections ADD COLUMN sync_status TEXT DEFAULT 'unknown';
ALTER TABLE calendar_connections ADD COLUMN sync_error TEXT;
