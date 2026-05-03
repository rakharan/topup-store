-- Add product_type column to distinguish diamond packages from subscriptions
ALTER TABLE products ADD COLUMN IF NOT EXISTS product_type VARCHAR(20) NOT NULL DEFAULT 'diamond';

-- Add check constraint
ALTER TABLE products ADD CONSTRAINT chk_product_type CHECK (product_type IN ('diamond', 'subscription', 'other'));

-- Backfill: mark known subscription products
UPDATE products SET product_type = 'subscription'
WHERE LOWER(name) LIKE '%weekly%'
   OR LOWER(name) LIKE '%monthly%'
   OR LOWER(name) LIKE '%pass%'
   OR LOWER(name) LIKE '%membership%';
