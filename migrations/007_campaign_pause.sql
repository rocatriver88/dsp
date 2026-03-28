-- Add auto-pause support: reason and timestamp for campaign pauses.
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS pause_reason TEXT;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS paused_at TIMESTAMPTZ;
