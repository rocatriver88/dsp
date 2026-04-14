-- Phase 1 schema: advertisers, campaigns, creatives

CREATE TABLE IF NOT EXISTS advertisers (
    id            BIGSERIAL PRIMARY KEY,
    company_name  TEXT NOT NULL,
    contact_email TEXT NOT NULL UNIQUE,
    api_key       TEXT NOT NULL UNIQUE,
    balance_cents BIGINT NOT NULL DEFAULT 0,
    billing_type  TEXT NOT NULL DEFAULT 'prepaid' CHECK (billing_type IN ('prepaid', 'postpaid')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS campaigns (
    id                BIGSERIAL PRIMARY KEY,
    advertiser_id     BIGINT NOT NULL REFERENCES advertisers(id),
    name              TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'paused', 'completed', 'deleted')),
    budget_total_cents BIGINT NOT NULL,
    budget_daily_cents BIGINT NOT NULL,
    spent_cents        BIGINT NOT NULL DEFAULT 0,
    bid_cpm_cents      INT NOT NULL,
    start_date         DATE,
    end_date           DATE,
    targeting          JSONB NOT NULL DEFAULT '{}',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS creatives (
    id              BIGSERIAL PRIMARY KEY,
    campaign_id     BIGINT NOT NULL REFERENCES campaigns(id),
    name            TEXT NOT NULL,
    format          TEXT NOT NULL CHECK (format IN ('banner', 'native', 'video')),
    size            TEXT NOT NULL,
    ad_markup       TEXT NOT NULL,
    destination_url TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_campaigns_advertiser ON campaigns(advertiser_id);
CREATE INDEX IF NOT EXISTS idx_campaigns_status ON campaigns(status);
CREATE INDEX IF NOT EXISTS idx_creatives_campaign ON creatives(campaign_id);
