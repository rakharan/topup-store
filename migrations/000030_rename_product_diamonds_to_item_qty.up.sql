DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'products'
          AND column_name = 'diamonds'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'products'
          AND column_name = 'item_qty'
    ) THEN
        ALTER TABLE products RENAME COLUMN diamonds TO item_qty;
    END IF;
END $$;

DROP INDEX IF EXISTS idx_products_game_diamonds;
CREATE INDEX IF NOT EXISTS idx_products_game_item_qty ON products(game, item_qty, is_active, deleted_at);
