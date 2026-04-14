-- Billing tables for Phase 4

CREATE TABLE IF NOT EXISTS transactions (
    id              BIGSERIAL PRIMARY KEY,
    advertiser_id   BIGINT NOT NULL REFERENCES advertisers(id),
    type            TEXT NOT NULL CHECK (type IN ('topup', 'spend', 'adjustment', 'refund')),
    amount_cents    BIGINT NOT NULL,
    balance_after   BIGINT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    reference_id    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invoices (
    id              BIGSERIAL PRIMARY KEY,
    advertiser_id   BIGINT NOT NULL REFERENCES advertisers(id),
    period_start    DATE NOT NULL,
    period_end      DATE NOT NULL,
    total_cents     BIGINT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'issued', 'paid', 'overdue')),
    issued_at       TIMESTAMPTZ,
    paid_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS daily_reconciliation (
    id              BIGSERIAL PRIMARY KEY,
    campaign_id     BIGINT NOT NULL REFERENCES campaigns(id),
    date            DATE NOT NULL,
    redis_spent     BIGINT NOT NULL,
    clickhouse_spent BIGINT NOT NULL,
    adjustment      BIGINT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'reconciled', 'adjusted')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(campaign_id, date)
);

CREATE INDEX IF NOT EXISTS idx_transactions_advertiser ON transactions(advertiser_id);
CREATE INDEX IF NOT EXISTS idx_invoices_advertiser ON invoices(advertiser_id);
CREATE INDEX IF NOT EXISTS idx_reconciliation_date ON daily_reconciliation(date);
