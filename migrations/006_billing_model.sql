-- Multi-billing model support: CPM, CPC, oCPM

ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS
    billing_model TEXT NOT NULL DEFAULT 'cpm' CHECK (billing_model IN ('cpm', 'cpc', 'ocpm'));

-- CPC: cost per click
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS
    bid_cpc_cents INT NOT NULL DEFAULT 0;

-- oCPM: optimized CPM (bid by CPA target, charge by CPM)
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS
    ocpm_target_cpa_cents INT NOT NULL DEFAULT 0;

-- Conversion tracking for oCPM
CREATE TABLE IF NOT EXISTS conversions (
    id              BIGSERIAL PRIMARY KEY,
    campaign_id     BIGINT NOT NULL REFERENCES campaigns(id),
    click_id        TEXT NOT NULL,
    conversion_type TEXT NOT NULL DEFAULT 'default',
    value_cents     BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conversions_campaign ON conversions(campaign_id);
CREATE INDEX IF NOT EXISTS idx_conversions_click ON conversions(click_id);
