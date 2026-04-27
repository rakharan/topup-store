-- Add digiflazz_ref_id column to decouple internal UUID from external ref_id
ALTER TABLE orders ADD COLUMN IF NOT EXISTS digiflazz_ref_id TEXT;

-- Backfill existing orders: use id as the digiflazz_ref_id
UPDATE orders SET digiflazz_ref_id = id::text WHERE digiflazz_ref_id IS NULL;

-- Make it NOT NULL for new orders
ALTER TABLE orders ALTER COLUMN digiflazz_ref_id SET NOT NULL;

-- Add unique index
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_digiflazz_ref_id ON orders(digiflazz_ref_id);
