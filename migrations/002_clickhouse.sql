-- Run this against ClickHouse (port 8123), not PostgreSQL
-- clickhouse-client --query "$(cat migrations/002_clickhouse.sql)"

CREATE TABLE IF NOT EXISTS bid_log (
  event_date     Date,
  event_time     DateTime,
  campaign_id    UInt64,
  creative_id    UInt64,
  advertiser_id  UInt64,
  exchange_id    String,
  request_id     String,
  geo_country    String,
  device_os      String,
  bid_price_cents UInt32,
  clear_price_cents UInt32,
  event_type     Enum8('bid'=1, 'win'=2, 'loss'=3, 'impression'=4, 'click'=5),
  loss_reason    String DEFAULT ''
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(event_date)
ORDER BY (campaign_id, event_date, event_time)
TTL event_date + INTERVAL 6 MONTH;
