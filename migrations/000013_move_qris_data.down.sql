-- Recreate columns on orders table (app no longer uses these, but needed for rollback)
ALTER TABLE orders ADD COLUMN IF NOT EXISTS qris_url TEXT;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS qris_image_base64 TEXT;

-- Copy data back from order_qris
UPDATE orders SET qris_url = oq.qris_url, qris_image_base64 = oq.qris_image_base64
FROM order_qris oq
WHERE orders.id = oq.order_id;

DROP TABLE IF EXISTS order_qris CASCADE;
