DROP INDEX IF EXISTS uq_rounds_one_final_per_event;

ALTER TABLE rounds
    DROP COLUMN IF EXISTS is_final;
