-- Add Google identity columns to hosts table for Google OAuth support
ALTER TABLE hosts ADD COLUMN google_id TEXT;
ALTER TABLE hosts ADD COLUMN google_email TEXT;

-- SQLite doesn't support ALTER COLUMN, but password_hash is already TEXT which accepts NULL/empty
-- No change needed for password_hash in SQLite

-- Unique index on google_id
CREATE UNIQUE INDEX idx_hosts_google_id ON hosts(google_id);
