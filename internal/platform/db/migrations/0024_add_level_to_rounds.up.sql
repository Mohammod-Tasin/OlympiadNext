-- Rounds become level-scoped: each of the three academic levels ("Junior",
-- "Secondary", "Higher Secondary" — the same enum users.level already
-- enforces, see internal/http/handler/profile_fields.go) gets its own
-- independent round sequence within an event, so "round 2" for Secondary
-- students is a different row from "round 2" for Junior students.
--
-- Backfill: two real rounds already exist in production for the current
-- active event. There is no prior level data to derive their level from,
-- so they are backfilled to 'Higher Secondary' as a placeholder — an
-- admin can recreate/reassign them per level after this migration if they
-- were meant to apply more broadly.
ALTER TABLE rounds ADD COLUMN IF NOT EXISTS level VARCHAR(50);
UPDATE rounds SET level = 'Higher Secondary' WHERE level IS NULL;
ALTER TABLE rounds ALTER COLUMN level SET NOT NULL;

-- Round ordering is now unique per (event, level) rather than per event:
-- each level has its own 1, 2, 3... sequence.
ALTER TABLE rounds DROP CONSTRAINT IF EXISTS uq_rounds_event_order;
ALTER TABLE rounds ADD CONSTRAINT uq_rounds_event_order UNIQUE (event_id, level, round_order);

-- Likewise, "one final round" is now per (event, level), not per event
-- overall — each level has its own final round.
DROP INDEX IF EXISTS uq_rounds_one_final_per_event;
CREATE UNIQUE INDEX IF NOT EXISTS uq_rounds_one_final_per_event
    ON rounds (event_id, level)
    WHERE is_final;
