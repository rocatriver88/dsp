-- Run this against ClickHouse (port 8123), not PostgreSQL
-- clickhouse-client --query "$(cat migrations/008_clickhouse_attribution.sql)"

-- Add device_id for attribution tracking (IDFA/GAID/OAID)
ALTER TABLE bid_log ADD COLUMN IF NOT EXISTS device_id String DEFAULT '' AFTER device_os;

-- Add charge_cents for profit calculation (advertiser charge per event)
ALTER TABLE bid_log ADD COLUMN IF NOT EXISTS charge_cents UInt32 DEFAULT 0 AFTER clear_price_cents;

-- Extend event_type enum to include conversion events
-- Note: ClickHouse supports ALTER TABLE MODIFY COLUMN to extend Enum8
ALTER TABLE bid_log MODIFY COLUMN event_type Enum8(
    'bid'=1,
    'win'=2,
    'loss'=3,
    'impression'=4,
    'click'=5,
    'conversion'=6
);
