-- Add reminder_sent field to bookings table
ALTER TABLE bookings ADD COLUMN reminder_sent INTEGER DEFAULT 0;

-- Create index for efficient reminder queries (upcoming confirmed bookings without reminders sent)
CREATE INDEX idx_bookings_reminder ON bookings(status, start_time, reminder_sent);
