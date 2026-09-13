-- #1898: track when a warning e-mail was last sent so the per-level cooldown
-- (重复频率) can suppress repeats while a warning stays unresolved.
ALTER TABLE warnings ADD COLUMN IF NOT EXISTS last_notified_at TIMESTAMP WITH TIME ZONE;
