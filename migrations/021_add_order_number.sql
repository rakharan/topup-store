-- Create daily_counters table for atomic order number generation
CREATE TABLE IF NOT EXISTS daily_counters (
    date DATE PRIMARY KEY,
    counter INT NOT NULL DEFAULT 0
);

-- Add order_number column to orders (nullable first for backfill)
ALTER TABLE orders ADD COLUMN IF NOT EXISTS order_number VARCHAR(20);

-- Backfill existing orders with order_number based on date and creation order
WITH numbered AS (
    SELECT id,
           'FT-' || TO_CHAR(created_at, 'YYYYMMDD') || '-' ||
           LPAD(ROW_NUMBER() OVER (PARTITION BY created_at::date ORDER BY created_at)::TEXT, 4, '0') AS new_order_number
    FROM orders
    WHERE order_number IS NULL
)
UPDATE orders
SET order_number = numbered.new_order_number
FROM numbered
WHERE orders.id = numbered.id;

-- Now make it NOT NULL and UNIQUE
ALTER TABLE orders ALTER COLUMN order_number SET NOT NULL;
ALTER TABLE orders ADD CONSTRAINT orders_order_number_unique UNIQUE (order_number);

-- Initialize today's counter to the max used counter for today
INSERT INTO daily_counters (date, counter)
SELECT CURRENT_DATE, COALESCE(MAX(
    CAST(SUBSTRING(order_number FROM 'FT-\d{8}-(\d{4})$') AS INTEGER)
), 0)
FROM orders
WHERE order_number LIKE 'FT-' || TO_CHAR(CURRENT_DATE, 'YYYYMMDD') || '-%'
ON CONFLICT (date) DO NOTHING;
