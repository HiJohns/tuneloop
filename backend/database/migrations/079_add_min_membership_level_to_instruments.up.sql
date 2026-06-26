ALTER TABLE instruments ADD COLUMN IF NOT EXISTS min_membership_level INTEGER REFERENCES membership_levels(id);
