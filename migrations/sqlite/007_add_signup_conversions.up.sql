-- Signup conversions table (SQLite version)
-- Tracks when booking invitees click the registration CTA and complete signup
CREATE TABLE signup_conversions (
    id TEXT PRIMARY KEY,
    source_booking_id TEXT REFERENCES bookings(id) ON DELETE SET NULL,
    invitee_email TEXT NOT NULL,
    clicked_at TEXT NOT NULL,
    registered_at TEXT,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Indexes for common query patterns
CREATE INDEX idx_signup_conversions_invitee_email ON signup_conversions(invitee_email);
CREATE INDEX idx_signup_conversions_tenant ON signup_conversions(tenant_id);
CREATE INDEX idx_signup_conversions_booking ON signup_conversions(source_booking_id);
CREATE INDEX idx_signup_conversions_clicked_at ON signup_conversions(clicked_at);
