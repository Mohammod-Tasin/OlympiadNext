-- Manual bKash/Nagad payment verification for exam registration.
--
-- A student sends the fee to the event's bKash/Nagad number, then submits
-- the transaction id here. An admin checks it against their bKash/Nagad
-- statement and approves or rejects; an approved row is the student's
-- confirmed registration for that event.
--
--   pending  — submitted, awaiting the admin's manual check
--   approved — admin matched the payment; the student is registered
--   rejected — admin could not match the payment

-- Per-event payment details. Kept on the event rather than a global
-- setting so each exam can carry its own merchant numbers and fee.
-- registration_fee is whole Bangladeshi Taka (BDT has no sub-unit in
-- practice for these payments); 0 means "no fee shown yet".
ALTER TABLE events
    ADD COLUMN IF NOT EXISTS bkash_number     VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS nagad_number     VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS registration_fee INTEGER     NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS exam_registrations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users (id),
    event_id       UUID NOT NULL REFERENCES events (id),
    payment_method VARCHAR(10)  NOT NULL,
    sender_number  VARCHAR(20)  NOT NULL,
    transaction_id VARCHAR(64)  NOT NULL,
    status         VARCHAR(10)  NOT NULL DEFAULT 'pending',
    reviewed_by    UUID REFERENCES users (id),
    reviewed_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_exam_reg_payment_method CHECK (payment_method IN ('bkash', 'nagad')),
    CONSTRAINT chk_exam_reg_status         CHECK (status IN ('pending', 'approved', 'rejected')),

    -- One transaction id can only ever back one registration: a resubmitted
    -- TrxID (by the same student or anyone else) is rejected outright.
    CONSTRAINT uq_exam_reg_transaction_id UNIQUE (transaction_id),
    -- A student registers for a given exam at most once.
    CONSTRAINT uq_exam_reg_user_event     UNIQUE (user_id, event_id)
);

-- The admin review queue filters on status, newest first.
CREATE INDEX IF NOT EXISTS idx_exam_reg_status ON exam_registrations (status, created_at DESC);
-- The student's "my registrations" lookup filters on user_id.
CREATE INDEX IF NOT EXISTS idx_exam_reg_user ON exam_registrations (user_id);
