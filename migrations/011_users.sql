-- 011_users.sql: User accounts for email+password authentication
CREATE TABLE IF NOT EXISTS users (
    id              BIGSERIAL PRIMARY KEY,
    email           TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    name            TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('platform_admin', 'advertiser')),
    advertiser_id   BIGINT REFERENCES advertisers(id) UNIQUE,  -- MVP: one user per advertiser
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    refresh_token_hash TEXT,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- advertiser role requires advertiser_id; platform_admin must NOT have one
ALTER TABLE users ADD CONSTRAINT users_role_advertiser_check
    CHECK ((role = 'advertiser' AND advertiser_id IS NOT NULL) OR
           (role = 'platform_admin' AND advertiser_id IS NULL));

CREATE INDEX idx_users_advertiser_id ON users(advertiser_id);

-- Add user_id tracking to audit_log
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES users(id);
