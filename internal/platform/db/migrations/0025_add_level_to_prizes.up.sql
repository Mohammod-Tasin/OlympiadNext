-- Prize tiers become level-scoped too, mirroring rounds: two different
-- levels may configure identical or overlapping rank ranges without
-- conflict (the overlap check in prizes.Service is scoped to
-- (event_id, level), not just event_id).
ALTER TABLE prizes ADD COLUMN IF NOT EXISTS level VARCHAR(50);
UPDATE prizes SET level = 'Higher Secondary' WHERE level IS NULL;
ALTER TABLE prizes ALTER COLUMN level SET NOT NULL;
