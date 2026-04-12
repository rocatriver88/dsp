-- migrations/010_phase3.sql
-- Phase 3: Invite codes + Audit log

-- Invite codes for controlled registration
CREATE TABLE IF NOT EXISTS invite_codes (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    created_by  TEXT NOT NULL DEFAULT 'system',
    max_uses    INT NOT NULL DEFAULT 1,
    used_count  INT NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add invite_code reference to registration requests
ALTER TABLE registration_requests
    ADD COLUMN IF NOT EXISTS invite_code TEXT;

-- Audit log for operation tracking
CREATE TABLE IF NOT EXISTS audit_log (
    id              BIGSERIAL PRIMARY KEY,
    advertiser_id   BIGINT,
    actor           TEXT NOT NULL,
    action          TEXT NOT NULL,
    resource_type   TEXT NOT NULL,
    resource_id     BIGINT,
    details         JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_advertiser ON audit_log(advertiser_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action, created_at DESC);
