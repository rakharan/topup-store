CREATE TABLE IF NOT EXISTS webhook_idempotency (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source TEXT NOT NULL CHECK (source IN ('midtrans', 'digiflazz')),
    signature TEXT NOT NULL,
    processed_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(source, signature)
);

CREATE INDEX IF NOT EXISTS idx_webhook_idempotency_lookup ON webhook_idempotency(source, signature);
