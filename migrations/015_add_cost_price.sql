-- Add cost price column for margin tracking
ALTER TABLE products ADD COLUMN IF NOT EXISTS cost_price_idr INT DEFAULT 0;

-- Set default cost_price_idr to 80% of selling price for existing products
UPDATE products SET cost_price_idr = (price_idr * 80 / 100) WHERE cost_price_idr = 0;
