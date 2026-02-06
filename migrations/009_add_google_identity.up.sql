-- Add Google identity columns to hosts table for Google OAuth support
ALTER TABLE hosts ADD COLUMN google_id VARCHAR(255);
ALTER TABLE hosts ADD COLUMN google_email VARCHAR(255);

-- Make password_hash nullable for Google-only accounts
ALTER TABLE hosts ALTER COLUMN password_hash DROP NOT NULL;

-- Unique constraint on google_id (only one host per Google account per tenant isn't enforced;
-- google_id is globally unique since it's a Google subject ID)
CREATE UNIQUE INDEX idx_hosts_google_id ON hosts(google_id) WHERE google_id IS NOT NULL;
