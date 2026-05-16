UPDATE products SET product_type = 'other' WHERE product_type = 'validation';

ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_product_type;

ALTER TABLE products
    ADD CONSTRAINT chk_product_type CHECK (product_type IN ('diamond', 'subscription', 'other'));
