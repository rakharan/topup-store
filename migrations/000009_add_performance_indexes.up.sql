CREATE INDEX IF NOT EXISTS idx_orders_uid_phone ON orders(game_uid, user_phone);
CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_products_game_active ON products(game, is_active);
