ALTER TABLE users ADD COLUMN IF NOT EXISTS ref_code VARCHAR(16);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_ref_code ON users(ref_code) WHERE ref_code IS NOT NULL AND ref_code <> '';

CREATE TABLE IF NOT EXISTS referrals (
    id          UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    referrer_id UUID NOT NULL REFERENCES users(id),
    referee_id  UUID NOT NULL REFERENCES users(id),
    ref_code    VARCHAR(16) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'registered',
    created_at  TIMESTAMP NOT NULL DEFAULT now(),
    UNIQUE(referee_id)
);
