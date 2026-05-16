CREATE TABLE IF NOT EXISTS referral_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(32) UNIQUE NOT NULL,
    discount_idr INT NOT NULL DEFAULT 0,
    max_uses INT NOT NULL DEFAULT 0,
    used_count INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS order_referrals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    code_id UUID NOT NULL REFERENCES referral_codes(id) ON DELETE CASCADE,
    discount_idr INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(order_id)
);

CREATE INDEX IF NOT EXISTS idx_referral_code ON referral_codes(code);
CREATE INDEX IF NOT EXISTS idx_order_referral_order ON order_referrals(order_id);
