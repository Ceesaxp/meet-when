-- Signup conversions table (PostgreSQL version)
-- Tracks when booking invitees click the registration CTA and complete signup
CREATE TABLE signup_conversions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_booking_id UUID REFERENCES bookings(id) ON DELETE SET NULL,
    invitee_email VARCHAR(255) NOT NULL,
    clicked_at TIMESTAMP WITH TIME ZONE NOT NULL,
    registered_at TIMESTAMP WITH TIME ZONE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for common query patterns
CREATE INDEX idx_signup_conversions_invitee_email ON signup_conversions(invitee_email);
CREATE INDEX idx_signup_conversions_tenant ON signup_conversions(tenant_id);
CREATE INDEX idx_signup_conversions_booking ON signup_conversions(source_booking_id);
CREATE INDEX idx_signup_conversions_clicked_at ON signup_conversions(clicked_at);

-- Add trigger for updated_at
CREATE TRIGGER update_signup_conversions_updated_at BEFORE UPDATE ON signup_conversions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
