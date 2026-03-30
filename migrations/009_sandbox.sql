-- Add sandbox flag to campaigns for testing without real budget
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS sandbox BOOLEAN NOT NULL DEFAULT FALSE;
