-- Create separate table for QRIS data to reduce orders table bloat
CREATE TABLE IF NOT EXISTS order_qris (
    order_id UUID PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE,
    qris_url TEXT,
    qris_image_base64 TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Migrate existing data
INSERT INTO order_qris (order_id, qris_url, qris_image_base64, created_at)
SELECT id, qris_url, qris_image_base64, NOW()
FROM orders
WHERE qris_url IS NOT NULL OR qris_image_base64 IS NOT NULL
ON CONFLICT (order_id) DO NOTHING;

-- Drop columns from orders table
ALTER TABLE orders DROP COLUMN IF EXISTS qris_url;
ALTER TABLE orders DROP COLUMN IF EXISTS qris_image_base64;
