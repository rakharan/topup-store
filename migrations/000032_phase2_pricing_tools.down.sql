ALTER TABLE products DROP COLUMN IF EXISTS benchmark_note;
ALTER TABLE products DROP COLUMN IF EXISTS competitor_price_idr;

ALTER TABLE orders DROP COLUMN IF EXISTS coupon_code;
ALTER TABLE orders DROP COLUMN IF EXISTS discount_idr;
ALTER TABLE orders DROP COLUMN IF EXISTS subtotal_idr;

DROP TABLE IF EXISTS coupons CASCADE;
