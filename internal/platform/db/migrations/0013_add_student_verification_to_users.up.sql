-- Manual student verification (KYC). Onboarding collects a proof document
-- and an optional profile picture; an admin then approves or rejects.
--   unverified — account created, no document submitted yet
--   pending    — document submitted, awaiting admin review
--   verified   — admin approved
--   rejected   — admin rejected; the user may resubmit (back to pending)

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS profile_picture     VARCHAR(255),
    ADD COLUMN IF NOT EXISTS verification_doc     VARCHAR(255),
    ADD COLUMN IF NOT EXISTS verification_status  VARCHAR(20) NOT NULL DEFAULT 'unverified';

-- Postgres has no ADD CONSTRAINT IF NOT EXISTS; guard it so a rerun is safe.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_users_verification_status'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT chk_users_verification_status
            CHECK (verification_status IN ('unverified', 'pending', 'verified', 'rejected'));
    END IF;
END $$;

-- The admin review queue filters on this column (?status=pending).
CREATE INDEX IF NOT EXISTS idx_users_verification_status ON users (verification_status);
