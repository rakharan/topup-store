-- Step 1: Backfill NULL values before adding NOT NULL constraint
UPDATE orders SET game_uid = 'unknown' WHERE game_uid IS NULL;
UPDATE orders SET user_phone = 'unknown' WHERE user_phone IS NULL;

-- Step 2: Add NOT NULL constraints
ALTER TABLE orders ALTER COLUMN game_uid SET NOT NULL;
ALTER TABLE orders ALTER COLUMN user_phone SET NOT NULL;

-- Step 3: Drop existing FK and recreate with ON DELETE RESTRICT
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_product_id_fkey;
ALTER TABLE orders ADD CONSTRAINT orders_product_id_fkey FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE RESTRICT;

-- Step 4: Add partial unique index on midtrans_order_id
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_midtrans_unique ON orders(midtrans_order_id) WHERE midtrans_order_id IS NOT NULL;
