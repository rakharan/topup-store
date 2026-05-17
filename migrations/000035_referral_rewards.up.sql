ALTER TABLE referral_codes ADD COLUMN IF NOT EXISTS owner_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE referral_codes ADD COLUMN IF NOT EXISTS reward_points INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS referral_point_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_phone TEXT NOT NULL,
    code_id UUID REFERENCES referral_codes(id) ON DELETE SET NULL,
    order_id UUID REFERENCES orders(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('earn', 'redeem', 'adjust')),
    points INT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(order_id)
);

CREATE INDEX IF NOT EXISTS idx_referral_point_ledger_owner ON referral_point_ledger(owner_phone);
CREATE INDEX IF NOT EXISTS idx_referral_codes_owner_phone ON referral_codes(owner_phone);
