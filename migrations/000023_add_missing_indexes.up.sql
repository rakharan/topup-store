-- Add missing indexes for common query patterns

-- For ListFailedForRetry: queries status='failed' with retry_count < N
CREATE INDEX IF NOT EXISTS idx_orders_status_retry ON orders(status, retry_count) WHERE status = 'failed';

-- For GetByGameAndDiamonds: queries game + diamonds with is_active=true AND deleted_at IS NULL
CREATE INDEX IF NOT EXISTS idx_products_game_diamonds ON products(game, diamonds, is_active, deleted_at);
