DROP INDEX IF EXISTS uq_rounds_one_final_per_event;
CREATE UNIQUE INDEX IF NOT EXISTS uq_rounds_one_final_per_event
    ON rounds (event_id)
    WHERE is_final;

ALTER TABLE rounds DROP CONSTRAINT IF EXISTS uq_rounds_event_order;
ALTER TABLE rounds ADD CONSTRAINT uq_rounds_event_order UNIQUE (event_id, round_order);

ALTER TABLE rounds DROP COLUMN IF EXISTS level;
