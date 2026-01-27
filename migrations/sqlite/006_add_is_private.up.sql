-- Add is_private column to meeting_templates for private/unlisted templates
ALTER TABLE meeting_templates ADD COLUMN is_private INTEGER DEFAULT 0;

-- Index for efficient filtering of public vs private templates
CREATE INDEX idx_meeting_templates_private ON meeting_templates(host_id, is_private);
