CREATE TABLE IF NOT EXISTS webhooks_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source TEXT NOT NULL CHECK (source IN ('digiflazz', 'midtrans')),
    ref_id TEXT,
    payload JSONB NOT NULL,
    signature TEXT,
    user_agent TEXT,
    status TEXT NOT NULL CHECK (status IN ('processed', 'skipped', 'failed')),
    error TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhooks_log_source_ref ON webhooks_log(source, ref_id);
CREATE INDEX IF NOT EXISTS idx_webhooks_log_created_at ON webhooks_log(created_at DESC);
