ALTER TYPE game_enum ADD VALUE IF NOT EXISTS 'pulsa';

ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_product_type;
ALTER TABLE products
    ADD CONSTRAINT chk_product_type CHECK (product_type IN ('diamond','subscription','other','validation','pulsa'));
