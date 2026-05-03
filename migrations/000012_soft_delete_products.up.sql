-- Add soft delete column to products
ALTER TABLE products ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Add partial index for active (non-deleted) products
CREATE INDEX IF NOT EXISTS idx_products_active ON products(game, is_active) WHERE deleted_at IS NULL;
