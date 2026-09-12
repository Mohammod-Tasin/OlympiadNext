-- Sequential rounds within an event (Qualifying/Semifinal/Final by
-- default, admin-renameable). Advancing from round N to round N+1
-- requires an explicit admin decision recorded in round_participants for
-- round N; round 1 draws its eligibility from exam_registrations instead
-- (see registration.ExistsApprovedForUserEvent), since there is no
-- "round 0" to hold a qualified decision.
--
--   upcoming — configured, not yet opened
--   ongoing  — students may attempt to enter (POST /rounds/{id}/enter)
--   ended    — closed; admin may now review candidates and decide

CREATE TABLE IF NOT EXISTS rounds (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id         UUID NOT NULL REFERENCES events (id),
    round_order      INTEGER NOT NULL,
    round_name       TEXT NOT NULL,
    start_at         TIMESTAMPTZ NOT NULL,
    duration_minutes INTEGER NOT NULL,
    status           VARCHAR(10) NOT NULL DEFAULT 'upcoming',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_rounds_status CHECK (status IN ('upcoming', 'ongoing', 'ended')),
    -- Rounds within one event are numbered without gaps or repeats.
    CONSTRAINT uq_rounds_event_order UNIQUE (event_id, round_order)
);

-- Listing an event's rounds in order is the only read pattern.
CREATE INDEX IF NOT EXISTS idx_rounds_event ON rounds (event_id, round_order);

-- A student's qualified/eliminated/winner decision for one round.
CREATE TABLE IF NOT EXISTS round_participants (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    round_id   UUID NOT NULL REFERENCES rounds (id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status     VARCHAR(12) NOT NULL,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_round_participants_status CHECK (status IN ('qualified', 'eliminated', 'winner')),
    -- One decision per student per round; a re-decision overwrites it.
    CONSTRAINT uq_round_participants_round_user UNIQUE (round_id, user_id)
);

-- The admin candidate/decision views and the round-N>1 eligibility check
-- both filter on round_id.
CREATE INDEX IF NOT EXISTS idx_round_participants_round ON round_participants (round_id);
