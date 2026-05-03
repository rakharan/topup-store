DROP INDEX IF EXISTS idx_products_active;
ALTER TABLE products DROP COLUMN IF EXISTS deleted_at;
