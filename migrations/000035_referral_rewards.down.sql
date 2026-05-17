DROP TABLE IF EXISTS referral_point_ledger;

ALTER TABLE referral_codes DROP COLUMN IF EXISTS reward_points;
ALTER TABLE referral_codes DROP COLUMN IF EXISTS owner_phone;
