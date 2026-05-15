-- Add missing indexes for common query patterns

-- For ListFailedForRetry: queries status='failed' with retry_count < N
CREATE INDEX IF NOT EXISTS idx_orders_status_retry ON orders(status, retry_count) WHERE status = 'failed';

-- For GetByGameAndDiamonds: queries game + item_qty with is_active=true AND deleted_at IS NULL
CREATE INDEX IF NOT EXISTS idx_products_game_item_qty ON products(game, item_qty, is_active, deleted_at);
