CREATE TABLE IF NOT EXISTS coupons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    discount_type TEXT NOT NULL CHECK (discount_type IN ('fixed', 'percent')),
    discount_value INT NOT NULL CHECK (discount_value > 0),
    min_order_idr INT NOT NULL DEFAULT 0,
    max_discount_idr INT NOT NULL DEFAULT 0,
    max_uses INT NOT NULL DEFAULT 0,
    used_count INT NOT NULL DEFAULT 0,
    game TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT true,
    starts_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_coupons_active_code ON coupons(code, is_active);

ALTER TABLE orders ADD COLUMN IF NOT EXISTS subtotal_idr INT NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS discount_idr INT NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS coupon_code TEXT NOT NULL DEFAULT '';
UPDATE orders SET subtotal_idr = amount_idr WHERE subtotal_idr = 0;

ALTER TABLE products ADD COLUMN IF NOT EXISTS competitor_price_idr INT NOT NULL DEFAULT 0;
ALTER TABLE products ADD COLUMN IF NOT EXISTS benchmark_note TEXT NOT NULL DEFAULT '';
