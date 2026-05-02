-- Track the most recent token-refresh failure for each conferencing connection.
-- When non-empty the connection is in a "reauth required" state.
ALTER TABLE conferencing_connections ADD COLUMN last_refresh_error TEXT;
