-- Marks an event's final round explicitly, rather than inferring it from
-- "highest round_order" at request time. At most one final round per
-- event: the partial unique index enforces this at the DB layer as a
-- backstop, but the service layer checks first so a violation surfaces as
-- a friendly 400, not a raw constraint error.
ALTER TABLE rounds
    ADD COLUMN IF NOT EXISTS is_final BOOLEAN NOT NULL DEFAULT false;

CREATE UNIQUE INDEX IF NOT EXISTS uq_rounds_one_final_per_event
    ON rounds (event_id)
    WHERE is_final;
