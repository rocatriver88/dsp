-- Self-service registration for Phase 4

ALTER TABLE advertisers ADD COLUMN IF NOT EXISTS
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('pending', 'active', 'suspended'));

ALTER TABLE advertisers ADD COLUMN IF NOT EXISTS
    credit_limit_cents BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS registration_requests (
    id              BIGSERIAL PRIMARY KEY,
    company_name    TEXT NOT NULL,
    contact_email   TEXT NOT NULL,
    contact_phone   TEXT,
    business_type   TEXT,
    website         TEXT,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewed_at     TIMESTAMPTZ,
    reviewed_by     TEXT,
    reject_reason   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_registration_status ON registration_requests(status);
