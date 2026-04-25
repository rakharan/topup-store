CREATE TYPE game_enum AS ENUM ('free_fire', 'mobile_legends', 'pubg_mobile');
CREATE TYPE order_status_enum AS ENUM ('pending', 'paid', 'processing', 'success', 'failed');
CREATE TYPE channel_enum AS ENUM ('whatsapp', 'web');

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game game_enum NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    price_idr INT NOT NULL,
    diamonds INT NOT NULL,
    sku TEXT NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID REFERENCES products(id),
    user_phone TEXT,
    game_uid TEXT,
    game_server TEXT,
    amount_idr INT NOT NULL,
    status order_status_enum DEFAULT 'pending',
    midtrans_order_id TEXT,
    qris_url TEXT,
    qris_image_base64 TEXT,
    channel channel_enum DEFAULT 'web',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_orders_midtrans ON orders(midtrans_order_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_products_game ON products(game);
