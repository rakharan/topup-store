-- Step 1: Reassign orders from duplicate products to the kept product (earliest ctid)
UPDATE orders SET product_id = kept.id
FROM products kept
JOIN products dup ON dup.sku = kept.sku AND dup.ctid > kept.ctid
WHERE orders.product_id = dup.id;

-- Step 2: Delete duplicate products
DELETE FROM products
WHERE ctid NOT IN (
    SELECT MIN(ctid)
    FROM products
    GROUP BY sku
);

-- Step 3: Add unique constraint if not already present
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'products_sku_unique') THEN
        ALTER TABLE products ADD CONSTRAINT products_sku_unique UNIQUE (sku);
    END IF;
END $$;
