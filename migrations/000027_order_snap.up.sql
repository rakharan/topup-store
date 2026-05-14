CREATE TABLE IF NOT EXISTS order_snap (
    order_id UUID PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE,
    snap_token TEXT NOT NULL,
    snap_redirect_url TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
