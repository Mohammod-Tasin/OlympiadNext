-- Admin-configured prize tiers for an event's final-round rankings, e.g.
-- "1st place" -> "Gold medal + BDT 5000", "2nd-3rd place" -> "Certificate".
-- Purely informational content for the client frontend; nothing here
-- feeds back into round_participants.rank or vice versa.
CREATE TABLE IF NOT EXISTS prizes (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id          UUID NOT NULL REFERENCES events (id),
    rank_from         INTEGER NOT NULL,
    rank_to           INTEGER NOT NULL,
    prize_name        TEXT NOT NULL,
    prize_description TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_prizes_rank_range CHECK (rank_from <= rank_to)
);

-- The admin and public prize-tier listings both filter by event and order
-- by rank_from.
CREATE INDEX IF NOT EXISTS idx_prizes_event ON prizes (event_id, rank_from);
