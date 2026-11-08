ALTER TABLE orders ADD COLUMN IF NOT EXISTS referral_level_id int;
CREATE TABLE IF NOT EXISTS referral_commissions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id uuid NOT NULL,
  referrer_id uuid NOT NULL,
  amount_cents bigint NOT NULL DEFAULT 0,
  level_id int,
  basis_cents bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_referral_commissions_order ON referral_commissions(order_id);
