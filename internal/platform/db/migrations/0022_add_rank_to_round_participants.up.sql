-- A winner's placement (1st, 2nd, 3rd...) on the event's final round.
-- Nullable: only ever set alongside status = 'winner' (enforced in
-- rounds.Service.SetParticipants, not the DB), so a qualified/eliminated
-- row always has rank NULL. The partial unique index only constrains rows
-- that do carry a rank, so two winners can never share a placement within
-- the same round.
ALTER TABLE round_participants
    ADD COLUMN IF NOT EXISTS rank INTEGER;

CREATE UNIQUE INDEX IF NOT EXISTS uq_round_participants_rank
    ON round_participants (round_id, rank)
    WHERE rank IS NOT NULL;
