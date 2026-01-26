-- Add sync status fields to calendar_connections table
ALTER TABLE calendar_connections ADD COLUMN last_synced_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE calendar_connections ADD COLUMN sync_status VARCHAR(20) DEFAULT 'unknown';
ALTER TABLE calendar_connections ADD COLUMN sync_error TEXT;
