ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_product_type;
ALTER TABLE products
    ADD CONSTRAINT chk_product_type CHECK (product_type IN ('diamond','subscription','other','validation','pulsa'));

UPDATE products SET deleted_at = NOW(), is_active = false WHERE game = 'gopay';
