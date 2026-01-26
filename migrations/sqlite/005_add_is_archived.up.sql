-- Add is_archived flag to bookings table
ALTER TABLE bookings ADD COLUMN is_archived INTEGER DEFAULT 0;

-- Create index for faster filtering of non-archived bookings
CREATE INDEX idx_bookings_is_archived ON bookings(is_archived);
