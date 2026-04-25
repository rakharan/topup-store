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

-- Step 3: Add unique constraint to prevent future duplicates
ALTER TABLE products ADD CONSTRAINT products_sku_unique UNIQUE (sku);
