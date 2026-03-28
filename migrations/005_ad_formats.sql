-- Add ad_type field to creatives for multi-format support
-- Types: splash (开屏), interstitial (插屏), native (原生), banner (横幅)

ALTER TABLE creatives ADD COLUMN IF NOT EXISTS
    ad_type TEXT NOT NULL DEFAULT 'banner' CHECK (ad_type IN ('splash', 'interstitial', 'native', 'banner'));

-- Native ads need structured fields instead of raw HTML markup
ALTER TABLE creatives ADD COLUMN IF NOT EXISTS
    native_title TEXT;
ALTER TABLE creatives ADD COLUMN IF NOT EXISTS
    native_desc TEXT;
ALTER TABLE creatives ADD COLUMN IF NOT EXISTS
    native_icon_url TEXT;
ALTER TABLE creatives ADD COLUMN IF NOT EXISTS
    native_image_url TEXT;
ALTER TABLE creatives ADD COLUMN IF NOT EXISTS
    native_cta TEXT;

-- Add ad_type index for filtering
CREATE INDEX IF NOT EXISTS idx_creatives_ad_type ON creatives(ad_type);
