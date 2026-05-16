CREATE TABLE IF NOT EXISTS blocked_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone TEXT,
    game_uid TEXT,
    ip_address TEXT,
    reason TEXT NOT NULL,
    blocked_by TEXT NOT NULL DEFAULT 'system',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_blocked_phone ON blocked_identities(phone);
CREATE INDEX IF NOT EXISTS idx_blocked_uid ON blocked_identities(game_uid);
CREATE INDEX IF NOT EXISTS idx_blocked_ip ON blocked_identities(ip_address);
