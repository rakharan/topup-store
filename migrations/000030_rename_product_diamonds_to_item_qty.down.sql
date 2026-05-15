DROP INDEX IF EXISTS idx_products_game_item_qty;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'products'
          AND column_name = 'item_qty'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'products'
          AND column_name = 'diamonds'
    ) THEN
        ALTER TABLE products RENAME COLUMN item_qty TO diamonds;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_products_game_diamonds ON products(game, diamonds, is_active, deleted_at);
