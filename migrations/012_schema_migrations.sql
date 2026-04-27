CREATE TABLE IF NOT EXISTS schema_migrations (
    version INT PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TIMESTAMPTZ DEFAULT NOW()
);

-- Seed with existing migration versions
INSERT INTO schema_migrations (version, name) VALUES
    (1, '001_init'),
    (2, '002_add_expired_status'),
    (3, '003_seed_products'),
    (4, '004_add_serial_number'),
    (5, '005_add_indexes'),
    (6, '006_add_product_updated_at'),
    (7, '007_deduplicate_products')
ON CONFLICT (version) DO NOTHING;
