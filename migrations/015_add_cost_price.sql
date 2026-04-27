-- Add cost price column for margin tracking
ALTER TABLE products ADD COLUMN IF NOT EXISTS cost_price_idr INT DEFAULT 0;

-- Set cost_price_idr based on real Digiflazz prices + 5% margin, rounded to nearest 100
-- Formula: round(price * 1.05 / 100) * 100
-- Update these values after running "Fetch Price List" in Postman and using "Sync Cost Prices"
UPDATE products SET cost_price_idr = 1900 WHERE sku = 'ff_12';
UPDATE products SET cost_price_idr = 6600 WHERE sku = 'ff_50';
UPDATE products SET cost_price_idr = 9400 WHERE sku = 'ff_70';
UPDATE products SET cost_price_idr = 19200 WHERE sku = 'ff_140';
UPDATE products SET cost_price_idr = 47000 WHERE sku = 'ff_355';
UPDATE products SET cost_price_idr = 1800 WHERE sku = 'ml_5';
UPDATE products SET cost_price_idr = 3000 WHERE sku = 'ml_10';
UPDATE products SET cost_price_idr = 3500 WHERE sku = 'ml_12';
