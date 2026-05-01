CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    filename TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed with all existing migration versions
INSERT INTO schema_migrations (version, filename) VALUES
    (1, '001_init.sql'),
    (2, '002_add_expired_status.sql'),
    (3, '003_seed_products.sql'),
    (4, '004_add_serial_number.sql'),
    (5, '005_add_indexes.sql'),
    (6, '006_add_product_updated_at.sql'),
    (7, '007_deduplicate_products.sql'),
    (8, '008_add_constraints.sql'),
    (9, '009_add_performance_indexes.sql'),
    (10, '010_add_digiflazz_ref_id.sql'),
    (11, '011_order_status_history.sql'),
    (12, '012_schema_migrations.sql'),
    (13, '013_soft_delete_products.sql'),
    (14, '014_move_qris_data.sql'),
    (15, '015_add_cost_price.sql'),
    (16, '016_webhooks_log.sql'),
    (17, '017_add_product_type.sql'),
    (18, '018_csrf_tokens.sql'),
    (19, '019_add_cancelled_status.sql'),
    (20, '020_webhook_retry_queue.sql')
ON CONFLICT (version) DO NOTHING;
