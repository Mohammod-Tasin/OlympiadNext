DROP INDEX IF EXISTS uq_round_participants_rank;

ALTER TABLE round_participants
    DROP COLUMN IF EXISTS rank;
