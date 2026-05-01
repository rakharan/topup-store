CREATE TABLE IF NOT EXISTS webhook_retry_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_log_id UUID REFERENCES webhooks_log(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('digiflazz', 'midtrans')),
    ref_id TEXT,
    payload JSONB NOT NULL,
    attempt INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    next_retry TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'dead')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_webhook_retry_queue_status_next_retry ON webhook_retry_queue(status, next_retry);
CREATE INDEX idx_webhook_retry_queue_created_at ON webhook_retry_queue(created_at);
